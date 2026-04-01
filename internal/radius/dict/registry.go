package dict

import (
	"fmt"
	"strings"

	"layeh.com/radius"
	"layeh.com/radius/dictionary"
)

const vendorSpecificType = radius.Type(26)

type AttrKey struct {
	VendorID uint32
	Type     uint32
}

type AttrDescriptor struct {
	Name       string
	VendorName string

	VendorID uint32
	Type     uint32

	ValueType dictionary.AttributeType

	IsVSA bool

	VendorTypeSize int
	VendorLenSize  int
}

type DictRegistry struct {
	byName     map[string]*AttrDescriptor
	byKey      map[AttrKey]*AttrDescriptor
	byVendorID map[uint32][]*AttrDescriptor
}

func NewRegistry(dict *dictionary.Dictionary) *DictRegistry {
	r := &DictRegistry{
		byName:     make(map[string]*AttrDescriptor),
		byKey:      make(map[AttrKey]*AttrDescriptor),
		byVendorID: make(map[uint32][]*AttrDescriptor),
	}
	if dict == nil {
		return r
	}

	for _, attr := range dict.Attributes {
		if desc := newStandardDescriptor(attr); desc != nil {
			r.add(desc)
		}
	}

	for _, vendor := range dict.Vendors {
		if vendor == nil {
			continue
		}
		for _, attr := range vendor.Attributes {
			if desc := newVendorDescriptor(vendor, attr); desc != nil {
				r.add(desc)
			}
		}
	}

	return r
}

func (r *DictRegistry) Lookup(name string) (*AttrDescriptor, error) {
	if r == nil {
		return nil, fmt.Errorf("radiusdict: nil registry")
	}
	desc := r.byName[strings.ToLower(strings.TrimSpace(name))]
	if desc == nil {
		return nil, fmt.Errorf("radiusdict: attribute %q not found", name)
	}
	return desc, nil
}

func (r *DictRegistry) LookupByAVP(avp *radius.AVP) (*AttrDescriptor, error) {
	if r == nil {
		return nil, fmt.Errorf("radiusdict: nil registry")
	}
	if avp == nil {
		return nil, fmt.Errorf("radiusdict: nil AVP")
	}

	if avp.Type != vendorSpecificType {
		desc := r.byKey[AttrKey{
			VendorID: 0,
			Type:     uint32(avp.Type),
		}]
		if desc == nil {
			return nil, fmt.Errorf("radiusdict: attribute (%d,%d) not found", 0, avp.Type)
		}
		return desc, nil
	}

	vendorID, vsa, err := radius.VendorSpecific(avp.Attribute)
	if err != nil {
		return nil, fmt.Errorf("radiusdict: parse VSA: %w", err)
	}

	descs := r.byVendorID[vendorID]
	for _, desc := range descs {
		vendorType, err := parseVendorType(desc, vsa)
		if err != nil {
			continue
		}
		if vendorType == desc.Type {
			return desc, nil
		}
	}

	return nil, fmt.Errorf("radiusdict: vendor attribute (%d,?) not found", vendorID)
}

func newStandardDescriptor(attr *dictionary.Attribute) *AttrDescriptor {
	if attr == nil || len(attr.OID) == 0 {
		return nil
	}
	return &AttrDescriptor{
		Name:      attr.Name,
		VendorID:  0,
		Type:      uint32(attr.OID[0]),
		ValueType: attr.Type,
	}
}

func newVendorDescriptor(vendor *dictionary.Vendor, attr *dictionary.Attribute) *AttrDescriptor {
	if vendor == nil || attr == nil || len(attr.OID) == 0 {
		return nil
	}
	return &AttrDescriptor{
		Name:           attr.Name,
		VendorName:     vendor.Name,
		VendorID:       uint32(vendor.Number),
		Type:           uint32(attr.OID[0]),
		ValueType:      attr.Type,
		IsVSA:          true,
		VendorTypeSize: vendor.GetTypeOctets(),
		VendorLenSize:  vendor.GetLengthOctets(),
	}
}

func (r *DictRegistry) add(desc *AttrDescriptor) {
	if desc == nil {
		return
	}

	key := AttrKey{
		VendorID: desc.VendorID,
		Type:     desc.Type,
	}
	if _, exists := r.byKey[key]; !exists {
		r.byKey[key] = desc
	}

	nameKey := strings.ToLower(desc.Name)
	if _, exists := r.byName[nameKey]; !exists {
		r.byName[nameKey] = desc
	}

	if desc.IsVSA {
		r.byVendorID[desc.VendorID] = append(r.byVendorID[desc.VendorID], desc)
	}
}
