// RadiusSE_MVP - minimal RADIUS test server (layeh/radius)
//
// How to run:
//  1. Install deps:
//     go mod tidy
//  2. Start server (UDP/1812):
//     go run .
//
// Example shared secret (default):
//
//	secret
//
// Dictionary:
//
//	loaded from ./dictionary (FreeRADIUS dictionary format)
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"layeh.com/radius"
	"layeh.com/radius/dictionary"
	"layeh.com/radius/rfc2865"
)

var radiusDict *dictionary.Dictionary

type xmlRoot struct {
	XMLName  xml.Name     `xml:"RadiusPacket"`
	Elements []xmlElement `xml:",any"`
}

type xmlElement struct {
	Name  string
	Value string
}

func sanitizeXMLTagName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Attribute"
	}

	runes := []rune(s)
	if len(runes) == 0 {
		return "Attribute"
	}

	// XML element names must start with a letter or underscore.
	var b strings.Builder
	if !(unicode.IsLetter(runes[0]) || runes[0] == '_') {
		b.WriteString("A_")
	}

	for _, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	return b.String()
}

func (e xmlElement) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	// Requirements: no XML attributes on tags; only tag name + text content.
	start.Attr = nil
	start.Name = xml.Name{Local: sanitizeXMLTagName(e.Name)}
	return enc.EncodeElement(e.Value, start)
}

func loadDictionary(dictRoot string) (*dictionary.Dictionary, error) {
	// Expect FreeRADIUS dictionary files:
	//   ./dictionary/dictionary/dictionary.rfc2865
	//   ./dictionary/dictionary/dictionary.rfc2866
	absRoot, err := filepath.Abs(dictRoot)
	if err != nil {
		return nil, fmt.Errorf("abs dict root: %w", err)
	}

	rfc2865 := filepath.Join(absRoot, "dictionary", "dictionary.rfc2865")
	rfc2866 := filepath.Join(absRoot, "dictionary", "dictionary.rfc2866")

	p := &dictionary.Parser{
		Opener: &dictionary.FileSystemOpener{Root: absRoot},
		// If there are duplicates across dictionary files, keep the first.
		IgnoreIdenticalAttributes: true,
	}

	d1, err := p.ParseFile(rfc2865)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rfc2865, err)
	}
	d2, err := p.ParseFile(rfc2866)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rfc2866, err)
	}

	merged, err := dictionary.Merge(d1, d2)
	if err != nil {
		return nil, fmt.Errorf("merge dictionaries: %w", err)
	}
	return merged, nil
}

func sidForAVP(avp *radius.AVP) string {
	if radiusDict == nil || avp == nil {
		return "#unknown"
	}

	dictAttr := dictionary.AttributeByOID(radiusDict.Attributes, dictionary.OID{int(avp.Type)})
	if dictAttr == nil {
		return fmt.Sprintf("#%d", int(avp.Type))
	}
	return dictAttr.Name
}

// attributeBytesToString converts unknown/unhandled attribute values into a stable hex representation.
// It is intentionally "universal" (doesn't try to detect the type).
func attributeBytesToString(attr radius.Attribute) string {
	return "0x" + hex.EncodeToString(attr)
}

func decimalFromBytesBE(attr radius.Attribute) string {
	// Big-endian unsigned integer, works for any byte length without losing data.
	var n big.Int
	n.SetBytes(attr)
	return n.String()
}

func attributeValueFromDictionaryType(attr radius.Attribute, typ dictionary.AttributeType) string {
	switch typ {
	case dictionary.AttributeIPAddr:
		// Strict: always present as standard IPv4 text if possible.
		ip := net.IP(attr)
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
		// Fallback: still avoid hex/byte-array output (best-effort).
		return ip.String()

	case dictionary.AttributeInteger, dictionary.AttributeDate, dictionary.AttributeInteger64:
		// Strict: decimal only (no hex, no byte array string).
		return decimalFromBytesBE(attr)

	case dictionary.AttributeString:
		// Strict: output UTF-8 as-is.
		return string(attr)

	default:
		// Strict: everything else is hex.
		return attributeBytesToString(attr)
	}
}

// AttributeValueToString converts an AVP value into a string using strict, dictionary-driven rules.
// It is independent of JSON/XML formatting.
func AttributeValueToString(avp *radius.AVP) string {
	if avp == nil {
		return "#unknown"
	}
	if radiusDict == nil {
		// Can't apply dictionary-based rules; use universal representation.
		return attributeBytesToString(avp.Attribute)
	}

	// Vendor-Specific Attribute: decode vendor id + vendor-type blocks, then convert each sub-attribute.
	if avp.Type == rfc2865.VendorSpecific_Type {
		vendorID, vsa, err := radius.VendorSpecific(avp.Attribute)
		if err != nil {
			return attributeBytesToString(avp.Attribute)
		}

		vendorLabel := fmt.Sprintf("vendor#%d", vendorID)
		var vendorDef *dictionary.Vendor
		for _, v := range radiusDict.Vendors {
			if v.Number == int(vendorID) {
				vendorDef = v
				vendorLabel = v.Name
				break
			}
		}

		parts := make([]string, 0, 4)
		for len(vsa) >= 3 {
			subTyp := vsa[0]
			subLen := int(vsa[1])

			if subLen < 3 || subLen > len(vsa) {
				// Malformed VSA tail; preserve bytes.
				parts = append(parts, fmt.Sprintf("tail=%s", attributeBytesToString(vsa)))
				break
			}

			subVal := vsa[2:subLen] // skip typ+len

			subName := fmt.Sprintf("#%d", subTyp)
			subStr := attributeBytesToString(subVal)
			if vendorDef != nil {
				if vendorAttr := dictionary.AttributeByOID(vendorDef.Attributes, dictionary.OID{int(subTyp)}); vendorAttr != nil {
					subName = vendorAttr.Name
					subStr = attributeValueFromDictionaryType(subVal, vendorAttr.Type)
				}
			}

			parts = append(parts, fmt.Sprintf("%s=%s", subName, subStr))
			vsa = vsa[subLen:]
		}
		if len(vsa) > 0 {
			parts = append(parts, fmt.Sprintf("tail=%s", attributeBytesToString(vsa)))
		}

		if len(parts) == 0 {
			return fmt.Sprintf("%s:%s", vendorLabel, attributeBytesToString(avp.Attribute))
		}
		return vendorLabel + ":" + strings.Join(parts, ";")
	}

	// Standard attribute.
	dictAttr := dictionary.AttributeByOID(radiusDict.Attributes, dictionary.OID{int(avp.Type)})
	if dictAttr == nil {
		return attributeBytesToString(avp.Attribute)
	}
	return attributeValueFromDictionaryType(avp.Attribute, dictAttr.Type)
}

// PacketToJSON converts a RADIUS packet into a JSON string.
//
// Format:
//
//	{
//	  "<SID>": "<value>",   // or "<SID>": ["<value1>", "<value2>"] if repeated
//	  ...
//	}
func PacketToJSON(packet *radius.Packet) (string, error) {
	if packet == nil {
		return "", fmt.Errorf("nil packet")
	}

	out := make(map[string]any, len(packet.Attributes))
	for _, avp := range packet.Attributes {
		if avp == nil {
			continue
		}
		sid := sidForAVP(avp)
		value := AttributeValueToString(avp)

		if prev, ok := out[sid]; ok {
			switch v := prev.(type) {
			case string:
				out[sid] = []string{v, value}
			case []string:
				out[sid] = append(v, value)
			default:
				out[sid] = []string{fmt.Sprintf("%v", v), value}
			}
		} else {
			out[sid] = value
		}
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PacketToXML converts a RADIUS packet into an XML string.
//
// Format:
//
//	<RadiusPacket>
//	  <User-Name>bob</User-Name>
//	  <State>0x...</State>
//	  ...
//	</RadiusPacket>
func PacketToXML(packet *radius.Packet) (string, error) {
	if packet == nil {
		return "", fmt.Errorf("nil packet")
	}

	elements := make([]xmlElement, 0, len(packet.Attributes))
	for _, avp := range packet.Attributes {
		if avp == nil {
			continue
		}
		elements = append(elements, xmlElement{
			Name:  sidForAVP(avp),
			Value: AttributeValueToString(avp),
		})
	}

	x := xmlRoot{
		Elements: elements,
	}
	b, err := xml.MarshalIndent(x, "", "  ")
	if err != nil {
		return "", err
	}

	// Keep output stable / readable for console logs.
	return xml.Header + string(b), nil
}

func main() {
	var (
		port     = flag.Int("port", 1812, "UDP port to listen on")
		secret   = flag.String("secret", "secret", "shared secret for RADIUS packets")
		dictRoot = flag.String("dict", "./dictionary", "path to FreeRADIUS dictionary root")
		sendTest = flag.Bool("send-test", false, "send one Access-Request to localhost and exit")
	)
	flag.Parse()

	secretBytes := []byte(*secret)

	if *sendTest {
		// Quick local smoke-test without external tools.
		req := radius.New(radius.CodeAccessRequest, secretBytes)

		userNameAttr, err := radius.NewString("test-user")
		if err != nil {
			log.Fatal(err)
		}
		req.Attributes.Add(radius.Type(1), userNameAttr) // User-Name = 1

		userPasswordAttr, err := radius.NewUserPassword(
			[]byte("test-password"),
			secretBytes,
			req.Authenticator[:],
		)
		if err != nil {
			log.Fatal(err)
		}
		req.Attributes.Add(radius.Type(2), userPasswordAttr) // User-Password = 2

		// Add some non-printable octets to demonstrate hex output.
		req.Attributes.Add(radius.Type(24), radius.Attribute{0x00, 0x01, 0x02, 0x7f}) // State = 24

		c := radius.Client{
			Net: "udp",
			// We are only checking that server replies correctly.
			InsecureSkipVerify: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		resp, err := c.Exchange(ctx, req, addr)
		if err != nil {
			log.Fatalf("send-test exchange failed: %v", err)
		}
		log.Printf("send-test got response code: %v", resp.Code.String())
		return
	}

	d, err := loadDictionary(*dictRoot)
	if err != nil {
		log.Fatalf("failed to load dictionary: %v", err)
	}
	radiusDict = d

	server := radius.PacketServer{
		Addr:         fmt.Sprintf(":%d", *port),
		Network:      "udp",
		SecretSource: radius.StaticSecretSource([]byte(*secret)),
		Handler: radius.HandlerFunc(func(w radius.ResponseWriter, r *radius.Request) {
			jsonStr, errJSON := PacketToJSON(r.Packet)
			if errJSON != nil {
				log.Printf("PacketToJSON error: %v", errJSON)
			} else {
				fmt.Println(jsonStr)
			}

			xmlStr, errXML := PacketToXML(r.Packet)
			if errXML != nil {
				log.Printf("PacketToXML error: %v", errXML)
			} else {
				fmt.Println(xmlStr)
			}

			// MVP requirement: always reply Access-Accept.
			if err := w.Write(r.Response(radius.CodeAccessAccept)); err != nil {
				log.Printf("failed to write response: %v", err)
			}
		}),
	}

	log.Printf("RADIUS MVP server listening on UDP :%d", *port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
