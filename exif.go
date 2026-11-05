package main

import (
	"encoding/binary"
	"math"
	"strings"
)

type tiffEntry struct {
	tag    uint16
	typ    uint16
	count  uint32
	offset uint32
	value  [4]byte
}

func parseJPEGExif(data []byte) map[string]any {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	offset := 2
	for offset+4 <= len(data) {
		if data[offset] != 0xFF {
			break
		}
		marker := data[offset+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			offset += 2
			continue
		}
		if marker == 0xD9 || marker == 0xDA {
			break
		}
		length := int(binary.BigEndian.Uint16(data[offset+2:]))
		if length < 2 || offset+2+length > len(data) {
			break
		}
		segment := data[offset+4 : offset+2+length]
		if marker == 0xE1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			return parseTIFF(segment[6:])
		}
		offset += 2 + length
	}
	return nil
}

func parseTIFF(data []byte) map[string]any {
	if len(data) < 8 {
		return nil
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil
	}
	if order.Uint16(data[2:4]) != 42 {
		return nil
	}
	ifd0 := int(order.Uint32(data[4:8]))
	entries, ok := readIFD(data, order, ifd0)
	if !ok {
		return nil
	}
	result := make(map[string]any)
	tagMap := map[uint16]string{
		0x010F: "Make", 0x0110: "Model", 0x0131: "Software",
		0x0132: "DateTime", 0x0112: "Orientation",
	}
	for tag, name := range tagMap {
		if value, ok := entryValue(data, order, entries[tag]); ok {
			result[name] = value
		}
	}
	if entry, ok := entries[0x8769]; ok {
		if offset, ok := asInt(entryValue(data, order, entry)); ok {
			if exifEntries, ok := readIFD(data, order, offset); ok {
				exifTags := map[uint16]string{
					0x9003: "DateTimeOriginal", 0x829A: "ExposureTime",
					0x829D: "FNumber", 0x8827: "ISO", 0x920A: "FocalLength",
				}
				for tag, name := range exifTags {
					if value, ok := entryValue(data, order, exifEntries[tag]); ok {
						result[name] = value
					}
				}
			}
		}
	}
	if entry, ok := entries[0x8825]; ok {
		if offset, ok := asInt(entryValue(data, order, entry)); ok {
			if gpsEntries, ok := readIFD(data, order, offset); ok {
				gpsTags := map[uint16]string{
					0x0001: "GPSLatitudeRef", 0x0002: "GPSLatitude",
					0x0003: "GPSLongitudeRef", 0x0004: "GPSLongitude",
				}
				for tag, name := range gpsTags {
					if value, ok := entryValue(data, order, gpsEntries[tag]); ok {
						result[name] = value
					}
				}
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func readIFD(data []byte, order binary.ByteOrder, offset int) (map[uint16]tiffEntry, bool) {
	if offset < 0 || offset+2 > len(data) {
		return nil, false
	}
	count := int(order.Uint16(data[offset:]))
	offset += 2
	entries := make(map[uint16]tiffEntry, count)
	for index := 0; index < count; index++ {
		start := offset + index*12
		if start+12 > len(data) {
			return nil, false
		}
		entry := tiffEntry{
			tag:    order.Uint16(data[start:]),
			typ:    order.Uint16(data[start+2:]),
			count:  order.Uint32(data[start+4:]),
			offset: order.Uint32(data[start+8:]),
		}
		copy(entry.value[:], data[start+8:start+12])
		entries[entry.tag] = entry
	}
	return entries, true
}

func entryValue(data []byte, order binary.ByteOrder, entry tiffEntry) (any, bool) {
	if entry.tag == 0 {
		return nil, false
	}
	size := typeSize(entry.typ)
	if size == 0 || entry.count == 0 || entry.count > 4096 {
		return nil, false
	}
	total := size * int(entry.count)
	var raw []byte
	if total <= 4 {
		raw = entry.value[:total]
	} else {
		offset := int(entry.offset)
		if offset < 0 || offset+total > len(data) {
			return nil, false
		}
		raw = data[offset : offset+total]
	}
	switch entry.typ {
	case 2:
		return strings.TrimRight(string(raw), "\x00"), true
	case 3:
		values := make([]uint16, entry.count)
		for index := range values {
			values[index] = order.Uint16(raw[index*2:])
		}
		if len(values) == 1 {
			return int(values[0]), true
		}
		return values, true
	case 4:
		values := make([]uint32, entry.count)
		for index := range values {
			values[index] = order.Uint32(raw[index*4:])
		}
		if len(values) == 1 {
			return int(values[0]), true
		}
		return values, true
	case 5:
		values := make([]float64, entry.count)
		for index := range values {
			numerator := order.Uint32(raw[index*8:])
			denominator := order.Uint32(raw[index*8+4:])
			if denominator == 0 {
				values[index] = 0
			} else {
				values[index] = float64(numerator) / float64(denominator)
			}
		}
		if len(values) == 1 {
			return values[0], true
		}
		return values, true
	case 7:
		return raw, true
	case 9:
		values := make([]int32, entry.count)
		for index := range values {
			values[index] = int32(order.Uint32(raw[index*4:]))
		}
		if len(values) == 1 {
			return int(values[0]), true
		}
		return values, true
	case 10:
		values := make([]float64, entry.count)
		for index := range values {
			numerator := int32(order.Uint32(raw[index*8:]))
			denominator := int32(order.Uint32(raw[index*8+4:]))
			if denominator == 0 {
				values[index] = 0
			} else {
				values[index] = float64(numerator) / float64(denominator)
			}
		}
		if len(values) == 1 {
			return values[0], true
		}
		return values, true
	default:
		return nil, false
	}
}

func typeSize(typ uint16) int {
	switch typ {
	case 1, 2, 7:
		return 1
	case 3:
		return 2
	case 4, 9:
		return 4
	case 5, 10:
		return 8
	default:
		return 0
	}
}

func asInt(value any, ok bool) (int, bool) {
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case uint32:
		return int(typed), true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}
