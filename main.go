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
	"flag"
	"fmt"
	"log"

	"github.com/Alexandr-Snisarenko/Radius_MVP/internal/radius/dict"
	"layeh.com/radius"
	"layeh.com/radius/dictionary"
)

var radiusDict *dictionary.Dictionary
var radiusRegistry *dict.DictRegistry

func loadDictionary(root string) (*dictionary.Dictionary, error) {
	parser := &dictionary.Parser{
		Opener: &dictionary.FileSystemOpener{
			Root: root,
		},
	}

	return parser.ParseFile("dictionary")
}

func main() {
	var (
		port     = flag.Int("port", 1812, "UDP port to listen on")
		secret   = flag.String("secret", "secret", "shared secret for RADIUS packets")
		dictRoot = flag.String("dict", "./dictionary", "path to FreeRADIUS dictionary root")
	)
	flag.Parse()
	d, err := loadDictionary(*dictRoot)
	if err != nil {
		log.Fatalf("failed to load dictionary: %v", err)
	}
	radiusDict = d
	radiusRegistry = dict.NewRegistry(d)

	server := radius.PacketServer{
		Addr:         fmt.Sprintf(":%d", *port),
		Network:      "udp",
		SecretSource: radius.StaticSecretSource([]byte(*secret)),
		Handler: radius.HandlerFunc(func(w radius.ResponseWriter, r *radius.Request) {
			jsonStr, errJSON := dict.WrapPacket(r.Packet, radiusRegistry).ToJSONIndent("", "  ")
			if errJSON != nil {
				log.Printf("radiusdict JSON marshal error: %v", errJSON)
			} else {
				log.Printf("received packet JSON:")
				fmt.Println(jsonStr)
			}

			// MVP requirement: always reply Access-Accept.
			response := r.Response(radius.CodeAccessAccept)
			nresp := dict.WrapPacket(response, radiusRegistry)
			nresp.AddAttribute("ERX-Virtual-Router-Name", "default")
			nresp.AddAttribute("NAS-Port-Type", 15)
			nresp.AddAttribute("Class", "12345")

			if err := w.Write(response); err != nil {
				log.Printf("failed to write response: %v", err)
			}
			log.Printf("RADIUS MVP server received request from %s", r.RemoteAddr)
			log.Printf("RADIUS MVP server received request code: %s", r.Code.String())

		}),
	}

	log.Printf("RADIUS MVP server listening on UDP :%d", *port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
