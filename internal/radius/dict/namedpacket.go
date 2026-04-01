package dict

import (
	"encoding/json"
	"fmt"

	"layeh.com/radius"
)

type NamedPacket struct {
	Packet *radius.Packet
	Dict   *DictRegistry
}

func WrapPacket(p *radius.Packet, d *DictRegistry) *NamedPacket {
	return &NamedPacket{
		Packet: p,
		Dict:   d,
	}
}

func (p *NamedPacket) GetAttribute(name string) ([]string, error) {
	if p == nil || p.Packet == nil {
		return nil, fmt.Errorf("radiusdict: nil packet")
	}
	desc, err := p.lookup(name)
	if err != nil {
		return nil, err
	}

	values := make([]string, 0, 1)
	for _, avp := range p.Packet.Attributes {
		raw, ok := p.matchValue(desc, avp)
		if !ok {
			continue
		}
		values = append(values, DecodeToString(desc, raw))
	}

	if len(values) == 0 {
		return nil, radius.ErrNoAttribute
	}
	return values, nil
}

func (p *NamedPacket) AddAttribute(name string, value any) error {
	if p == nil || p.Packet == nil {
		return fmt.Errorf("radiusdict: nil packet")
	}
	desc, err := p.lookup(name)
	if err != nil {
		return err
	}

	raw, err := EncodeValue(desc, value)
	if err != nil {
		return err
	}

	if !desc.IsVSA {
		p.Packet.Attributes.Add(radius.Type(desc.Type), raw)
		return nil
	}

	vendorPayload, err := buildVendorAttribute(desc, raw)
	if err != nil {
		return err
	}
	attr, err := radius.NewVendorSpecific(desc.VendorID, vendorPayload)
	if err != nil {
		return err
	}
	p.Packet.Attributes.Add(vendorSpecificType, attr)
	return nil
}

func (p *NamedPacket) SetAttribute(name string, value any) error {
	p.DelAttribute(name)
	return p.AddAttribute(name, value)
}

func (p *NamedPacket) DelAttribute(name string) {
	if p == nil || p.Packet == nil || p.Dict == nil {
		return
	}
	desc, err := p.Dict.Lookup(name)
	if err != nil {
		return
	}

	filtered := p.Packet.Attributes[:0]
	for _, avp := range p.Packet.Attributes {
		if _, ok := p.matchValue(desc, avp); ok {
			continue
		}
		filtered = append(filtered, avp)
	}
	p.Packet.Attributes = filtered
}

func (p *NamedPacket) ToMap() (map[string]any, error) {
	if p == nil || p.Packet == nil {
		return nil, fmt.Errorf("radiusdict: nil packet")
	}
	if p.Dict == nil {
		return nil, fmt.Errorf("radiusdict: nil registry")
	}

	grouped := make(map[string][]string)
	for _, avp := range p.Packet.Attributes {
		if avp == nil {
			continue
		}

		desc, err := p.Dict.LookupByAVP(avp)
		if err != nil {
			continue
		}

		raw, ok := p.matchValue(desc, avp)
		if !ok {
			continue
		}
		grouped[desc.Name] = append(grouped[desc.Name], DecodeToString(desc, raw))
	}

	out := make(map[string]any, len(grouped))
	for name, values := range grouped {
		if len(values) == 1 {
			out[name] = values[0]
			continue
		}
		out[name] = values
	}
	return out, nil
}

func (p *NamedPacket) MarshalJSON() ([]byte, error) {
	m, err := p.ToMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (p *NamedPacket) MarshalJSONIndent(prefix, indent string) ([]byte, error) {
	m, err := p.ToMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, prefix, indent)
}

func (p *NamedPacket) ToJSON() (string, error) {
	b, err := p.MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (p *NamedPacket) ToJSONIndent(prefix, indent string) (string, error) {
	b, err := p.MarshalJSONIndent(prefix, indent)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (p *NamedPacket) lookup(name string) (*AttrDescriptor, error) {
	if p == nil || p.Dict == nil {
		return nil, fmt.Errorf("radiusdict: nil registry")
	}
	return p.Dict.Lookup(name)
}

func (p *NamedPacket) matchValue(desc *AttrDescriptor, avp *radius.AVP) (radius.Attribute, bool) {
	if desc == nil || avp == nil {
		return nil, false
	}

	if !desc.IsVSA {
		if avp.Type != radius.Type(desc.Type) {
			return nil, false
		}
		return avp.Attribute, true
	}

	if avp.Type != vendorSpecificType {
		return nil, false
	}

	vendorID, vsa, err := radius.VendorSpecific(avp.Attribute)
	if err != nil || vendorID != desc.VendorID {
		return nil, false
	}

	vendorType, err := parseVendorType(desc, vsa)
	if err != nil || vendorType != desc.Type {
		return nil, false
	}

	raw, err := extractVendorValue(desc, vsa)
	if err != nil {
		return nil, false
	}
	return raw, true
}
