package dict

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"layeh.com/radius"
	"layeh.com/radius/dictionary"
)

func DecodeToString(desc *AttrDescriptor, raw radius.Attribute) string {
	if desc == nil {
		return encodeHex(raw)
	}

	switch desc.ValueType {
	case dictionary.AttributeIPAddr:
		ip, err := radius.IPAddr(raw)
		if err != nil {
			return encodeHex(raw)
		}
		return ip.String()
	case dictionary.AttributeIPv6Addr:
		ip, err := radius.IPv6Addr(raw)
		if err != nil {
			return encodeHex(raw)
		}
		return ip.String()
	case dictionary.AttributeInteger:
		v, err := radius.Integer(raw)
		if err != nil {
			return encodeHex(raw)
		}
		return strconv.FormatUint(uint64(v), 10)
	case dictionary.AttributeDate:
		v, err := radius.Date(raw)
		if err != nil {
			return encodeHex(raw)
		}
		return strconv.FormatInt(v.Unix(), 10)
	case dictionary.AttributeInteger64:
		v, err := radius.Integer64(raw)
		if err != nil {
			return encodeHex(raw)
		}
		return strconv.FormatUint(v, 10)
	case dictionary.AttributeString:
		return radius.String(raw)
	default:
		return encodeHex(raw)
	}
}

func EncodeValue(desc *AttrDescriptor, value any) (radius.Attribute, error) {
	if desc == nil {
		return nil, fmt.Errorf("radiusdict: nil descriptor")
	}

	switch desc.ValueType {
	case dictionary.AttributeString:
		return radius.NewString(fmt.Sprint(value))
	case dictionary.AttributeIPAddr:
		ip, err := coerceIP(value)
		if err != nil {
			return nil, err
		}
		return radius.NewIPAddr(ip)
	case dictionary.AttributeIPv6Addr:
		ip, err := coerceIP(value)
		if err != nil {
			return nil, err
		}
		return radius.NewIPv6Addr(ip)
	case dictionary.AttributeInteger:
		v, err := coerceUint32(value)
		if err != nil {
			return nil, err
		}
		return radius.NewInteger(v), nil
	case dictionary.AttributeDate:
		switch v := value.(type) {
		case time.Time:
			return radius.NewDate(v)
		default:
			n, err := coerceUint32(value)
			if err != nil {
				return nil, err
			}
			return radius.NewDate(time.Unix(int64(n), 0))
		}
	case dictionary.AttributeInteger64:
		v, err := coerceUint64(value)
		if err != nil {
			return nil, err
		}
		return radius.NewInteger64(v), nil
	default:
		raw, err := coerceRaw(value)
		if err != nil {
			return nil, err
		}
		return radius.NewBytes(raw)
	}
}

func coerceIP(value any) (net.IP, error) {
	switch v := value.(type) {
	case net.IP:
		return append(net.IP(nil), v...), nil
	case string:
		ip := net.ParseIP(strings.TrimSpace(v))
		if ip == nil {
			return nil, fmt.Errorf("radiusdict: invalid IP %q", v)
		}
		return ip, nil
	default:
		return nil, fmt.Errorf("radiusdict: unsupported IP value %T", value)
	}
}

func coerceUint32(value any) (uint32, error) {
	switch v := value.(type) {
	case uint8:
		return uint32(v), nil
	case uint16:
		return uint32(v), nil
	case uint32:
		return v, nil
	case uint64:
		if v > uint64(^uint32(0)) {
			return 0, fmt.Errorf("radiusdict: value %d overflows uint32", v)
		}
		return uint32(v), nil
	case uint:
		if uint64(v) > uint64(^uint32(0)) {
			return 0, fmt.Errorf("radiusdict: value %d overflows uint32", v)
		}
		return uint32(v), nil
	case int8:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint32(v), nil
	case int16:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint32(v), nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint32(v), nil
	case int64:
		if v < 0 || v > int64(^uint32(0)) {
			return 0, fmt.Errorf("radiusdict: invalid uint32 value %d", v)
		}
		return uint32(v), nil
	case int:
		if v < 0 || uint64(v) > uint64(^uint32(0)) {
			return 0, fmt.Errorf("radiusdict: invalid uint32 value %d", v)
		}
		return uint32(v), nil
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err != nil {
			return 0, fmt.Errorf("radiusdict: parse uint32 %q: %w", v, err)
		}
		return uint32(n), nil
	default:
		return 0, fmt.Errorf("radiusdict: unsupported uint32 value %T", value)
	}
}

func coerceUint64(value any) (uint64, error) {
	switch v := value.(type) {
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	case uint:
		return uint64(v), nil
	case int8:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint64(v), nil
	case int16:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint64(v), nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint64(v), nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("radiusdict: negative value %d", v)
		}
		return uint64(v), nil
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("radiusdict: parse uint64 %q: %w", v, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("radiusdict: unsupported uint64 value %T", value)
	}
}

func coerceRaw(value any) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return append([]byte(nil), v...), nil
	case radius.Attribute:
		return append([]byte(nil), v...), nil
	case string:
		text := strings.TrimSpace(v)
		if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
			b, err := hex.DecodeString(text[2:])
			if err != nil {
				return nil, fmt.Errorf("radiusdict: decode hex %q: %w", v, err)
			}
			return b, nil
		}
		return []byte(text), nil
	default:
		return nil, fmt.Errorf("radiusdict: unsupported raw value %T", value)
	}
}

func encodeHex(raw radius.Attribute) string {
	return "0x" + hex.EncodeToString(raw)
}
