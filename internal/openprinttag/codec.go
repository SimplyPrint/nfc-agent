package openprinttag

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
)

// CBOR encoding/decoding modes
var (
	decMode cbor.DecMode
	// encMode is used for encoding individual values, but we use custom encoding for maps
	encMode cbor.EncMode
)

func init() {
	var err error

	// Configure encoder for individual value encoding (not maps)
	encMode, err = cbor.EncOptions{
		Sort:        cbor.SortCanonical,
		IndefLength: cbor.IndefLengthAllowed,
	}.EncMode()
	if err != nil {
		panic(fmt.Sprintf("failed to create CBOR encoder: %v", err))
	}

	// Configure decoder
	decMode, err = cbor.DecOptions{
		IntDec: cbor.IntDecConvertSigned,
	}.DecMode()
	if err != nil {
		panic(fmt.Sprintf("failed to create CBOR decoder: %v", err))
	}
}

// encodeIndefiniteMap encodes a map with integer keys as an indefinite-length CBOR map.
// Per OpenPrintTag spec: "CBOR maps and arrays SHOULD be encoded as indefinite containers."
func encodeIndefiniteMap(keyValuePairs []keyValue) ([]byte, error) {
	var buf bytes.Buffer

	// Sort by key (integer keys)
	sort.Slice(keyValuePairs, func(i, j int) bool {
		return keyValuePairs[i].key < keyValuePairs[j].key
	})

	// Start indefinite map (0xbf)
	buf.WriteByte(0xbf)

	for _, kv := range keyValuePairs {
		// Encode key as unsigned integer
		keyBytes, err := encMode.Marshal(kv.key)
		if err != nil {
			return nil, fmt.Errorf("failed to encode key %d: %w", kv.key, err)
		}
		buf.Write(keyBytes)

		// Encode value
		valBytes, err := encMode.Marshal(kv.value)
		if err != nil {
			return nil, fmt.Errorf("failed to encode value for key %d: %w", kv.key, err)
		}
		buf.Write(valBytes)
	}

	// End indefinite map (0xff = "break")
	buf.WriteByte(0xff)

	return buf.Bytes(), nil
}

type keyValue struct {
	key   int
	value interface{}
}

// Decode parses CBOR payload into OpenPrintTag structure.
// The payload contains concatenated CBOR sections: Meta + Main + Aux
// The meta section specifies byte offsets to the other sections.
func Decode(payload []byte) (*OpenPrintTag, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	opt := &OpenPrintTag{}

	// First, try to decode the meta section as a generic map to check its structure
	// Meta section only contains integer values (offsets), while Main section contains
	// UUIDs (byte strings) for key 2. This lets us distinguish between them.
	var firstMap map[int]interface{}
	metaDecoder := decMode.NewDecoder(bytes.NewReader(payload))
	if err := metaDecoder.Decode(&firstMap); err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Check if this looks like a meta section (key 2 would be an integer offset)
	// or a main section (key 2 would be a byte string for MaterialUUID)
	auxOffsetVal, hasAuxOffset := firstMap[2]
	isMetaSection := false
	var auxOffset int

	if hasAuxOffset {
		// If key 2 is an integer, it's the aux_region_offset in meta section
		switch v := auxOffsetVal.(type) {
		case int64:
			isMetaSection = true
			auxOffset = int(v)
		case uint64:
			isMetaSection = true
			auxOffset = int(v)
		case int:
			isMetaSection = true
			auxOffset = v
		case []byte:
			// It's a byte string (UUID), so this is actually a Main section
			isMetaSection = false
		}
	}

	if !isMetaSection {
		// The payload is just a Main section without meta
		mainDecoder := decMode.NewDecoder(bytes.NewReader(payload))
		if err := mainDecoder.Decode(&opt.Main); err != nil {
			return nil, fmt.Errorf("failed to decode main section: %w", err)
		}
		return opt, nil
	}

	// We have a meta section - decode the structure properly
	opt.Meta.AuxOffset = uint16(auxOffset)
	if mainOffsetVal, ok := firstMap[0]; ok {
		switch v := mainOffsetVal.(type) {
		case int64:
			opt.Meta.MainOffset = uint16(v)
		case uint64:
			opt.Meta.MainOffset = uint16(v)
		case int:
			opt.Meta.MainOffset = uint16(v)
		}
	}
	if mainSizeVal, ok := firstMap[1]; ok {
		switch v := mainSizeVal.(type) {
		case int64:
			opt.Meta.MainSize = uint16(v)
		case uint64:
			opt.Meta.MainSize = uint16(v)
		case int:
			opt.Meta.MainSize = uint16(v)
		}
	}
	if auxSizeVal, ok := firstMap[3]; ok {
		switch v := auxSizeVal.(type) {
		case int64:
			opt.Meta.AuxSize = uint16(v)
		case uint64:
			opt.Meta.AuxSize = uint16(v)
		case int:
			opt.Meta.AuxSize = uint16(v)
		}
	}

	// Calculate where meta section ends by re-encoding to find its size
	// Or use the MainOffset if specified, otherwise find the next CBOR value
	metaEnd := 0
	if opt.Meta.MainOffset > 0 {
		metaEnd = int(opt.Meta.MainOffset)
	} else {
		// Re-encode the meta map to find its size
		metaBytes, _ := encMode.Marshal(firstMap)
		metaEnd = len(metaBytes)
	}

	// Decode Main section - starts right after meta
	mainStart := metaEnd
	mainEnd := int(opt.Meta.AuxOffset)
	if mainEnd > len(payload) {
		mainEnd = len(payload)
	}
	if mainStart < mainEnd {
		mainData := payload[mainStart:mainEnd]
		if err := decMode.Unmarshal(mainData, &opt.Main); err != nil {
			return nil, fmt.Errorf("failed to decode main section: %w", err)
		}
	}

	// Decode Auxiliary section
	if int(opt.Meta.AuxOffset) < len(payload) {
		auxData := payload[opt.Meta.AuxOffset:]
		if err := decMode.Unmarshal(auxData, &opt.Aux); err != nil {
			// Aux section might be empty or malformed, ignore error
		}
	}

	return opt, nil
}

// Encode serializes OpenPrintTag to CBOR bytes for NDEF payload.
// Returns concatenated CBOR: Meta + Main + Aux
// Per OpenPrintTag spec: "CBOR maps and arrays SHOULD be encoded as indefinite containers."
func (o *OpenPrintTag) Encode() ([]byte, error) {
	// Encode main section as indefinite-length map
	mainKV := o.Main.toKeyValuePairs()
	mainBytes, err := encodeIndefiniteMap(mainKV)
	if err != nil {
		return nil, fmt.Errorf("failed to encode main section: %w", err)
	}
	if len(mainBytes) > MaxSectionSize {
		return nil, fmt.Errorf("main section exceeds %d bytes (got %d)", MaxSectionSize, len(mainBytes))
	}

	// Encode auxiliary section as indefinite-length map
	auxKV := o.Aux.toKeyValuePairs()
	auxBytes, err := encodeIndefiniteMap(auxKV)
	if err != nil {
		return nil, fmt.Errorf("failed to encode auxiliary section: %w", err)
	}
	if len(auxBytes) > MaxSectionSize {
		return nil, fmt.Errorf("auxiliary section exceeds %d bytes (got %d)", MaxSectionSize, len(auxBytes))
	}

	// Meta section: only include aux_region_offset (key 2) per simplified format
	// Main region starts immediately after meta, so we only need to specify where aux starts
	metaKV := []keyValue{
		{key: 2, value: len(mainBytes)}, // aux_region_offset = size of main section (relative to start of main)
	}

	// We need to determine meta size iteratively since CBOR uses variable-length encoding
	var metaBytes []byte
	for iteration := 0; iteration < 5; iteration++ {
		estimatedMetaSize := 4 // typical for {2: small_int}
		if metaBytes != nil {
			estimatedMetaSize = len(metaBytes)
		}

		// Update aux offset to account for meta size
		metaKV[0].value = estimatedMetaSize + len(mainBytes)

		// Encode meta (can be definite-length since it's small)
		metaBytes, err = encMode.Marshal(map[int]int{2: metaKV[0].value.(int)})
		if err != nil {
			return nil, fmt.Errorf("failed to encode meta section: %w", err)
		}

		if len(metaBytes) == estimatedMetaSize {
			break
		}
	}

	// Combine all sections
	result := make([]byte, 0, len(metaBytes)+len(mainBytes)+len(auxBytes))
	result = append(result, metaBytes...)
	result = append(result, mainBytes...)
	result = append(result, auxBytes...)

	return result, nil
}

// toKeyValuePairs converts MainSection to key-value pairs for CBOR encoding
func (m *MainSection) toKeyValuePairs() []keyValue {
	var kv []keyValue

	// UUIDs (keys 0-3)
	if len(m.InstanceUUID) > 0 {
		kv = append(kv, keyValue{0, m.InstanceUUID})
	}
	if len(m.PackageUUID) > 0 {
		kv = append(kv, keyValue{1, m.PackageUUID})
	}
	if len(m.MaterialUUID) > 0 {
		kv = append(kv, keyValue{2, m.MaterialUUID})
	}
	if len(m.BrandUUID) > 0 {
		kv = append(kv, keyValue{3, m.BrandUUID})
	}

	// Identifiers (keys 4-7)
	if m.GTIN != 0 {
		kv = append(kv, keyValue{4, m.GTIN})
	}
	if m.BrandSpecificInstanceID != "" {
		kv = append(kv, keyValue{5, m.BrandSpecificInstanceID})
	}
	if m.BrandSpecificPackageID != "" {
		kv = append(kv, keyValue{6, m.BrandSpecificPackageID})
	}
	if m.BrandSpecificMaterialID != "" {
		kv = append(kv, keyValue{7, m.BrandSpecificMaterialID})
	}

	// Material classification (keys 8-11)
	// Note: MaterialClass and MaterialType are included even if 0 (FFF/PLA)
	kv = append(kv, keyValue{8, uint8(m.MaterialClass)})
	kv = append(kv, keyValue{9, uint8(m.MaterialType)})
	if m.MaterialName != "" {
		kv = append(kv, keyValue{10, m.MaterialName})
	}
	if m.BrandName != "" {
		kv = append(kv, keyValue{11, m.BrandName})
	}

	// Protection and dates (keys 13-15)
	if m.WriteProtection != 0 {
		kv = append(kv, keyValue{13, m.WriteProtection})
	}
	if m.ManufacturedDate != 0 {
		kv = append(kv, keyValue{14, m.ManufacturedDate})
	}
	if m.ExpirationDate != 0 {
		kv = append(kv, keyValue{15, m.ExpirationDate})
	}

	// Weight data (keys 16-18)
	if m.NominalNettoFullWeight != 0 {
		kv = append(kv, keyValue{16, m.NominalNettoFullWeight})
	}
	if m.ActualNettoFullWeight != 0 {
		kv = append(kv, keyValue{17, m.ActualNettoFullWeight})
	}
	if m.EmptyContainerWeight != 0 {
		kv = append(kv, keyValue{18, m.EmptyContainerWeight})
	}

	// Colors (keys 19-21)
	if len(m.PrimaryColor) > 0 {
		kv = append(kv, keyValue{19, m.PrimaryColor})
	}
	if len(m.SecondaryColor) > 0 {
		kv = append(kv, keyValue{20, m.SecondaryColor})
	}
	if len(m.TertiaryColor) > 0 {
		kv = append(kv, keyValue{21, m.TertiaryColor})
	}

	// Tags (key 28)
	if len(m.Tags) > 0 {
		kv = append(kv, keyValue{28, m.Tags})
	}

	// Physical properties (keys 29-31)
	if m.Density != 0 {
		kv = append(kv, keyValue{29, m.Density})
	}
	if m.FilamentDiameter != 0 {
		kv = append(kv, keyValue{30, m.FilamentDiameter})
	}
	if m.ShoreHardnessA != 0 {
		kv = append(kv, keyValue{31, m.ShoreHardnessA})
	}

	// Temperature settings (keys 34-36)
	if m.MinPrintTemp != 0 {
		kv = append(kv, keyValue{34, m.MinPrintTemp})
	}
	if m.MaxPrintTemp != 0 {
		kv = append(kv, keyValue{35, m.MaxPrintTemp})
	}
	if m.PreheatTemp != 0 {
		kv = append(kv, keyValue{36, m.PreheatTemp})
	}

	// Additional metadata (keys 52-56)
	if m.MaterialAbbreviation != "" {
		kv = append(kv, keyValue{52, m.MaterialAbbreviation})
	}
	if m.CountryOfOrigin != "" {
		kv = append(kv, keyValue{55, m.CountryOfOrigin})
	}
	if len(m.Certifications) > 0 {
		kv = append(kv, keyValue{56, m.Certifications})
	}

	return kv
}

// toKeyValuePairs converts AuxSection to key-value pairs for CBOR encoding
func (a *AuxSection) toKeyValuePairs() []keyValue {
	var kv []keyValue

	if a.ConsumedWeight != 0 {
		kv = append(kv, keyValue{0, a.ConsumedWeight})
	}
	if a.Workgroup != "" {
		kv = append(kv, keyValue{1, a.Workgroup})
	}
	if a.GeneralPurposeUser != "" {
		kv = append(kv, keyValue{2, a.GeneralPurposeUser})
	}
	if a.LastStirTime != 0 {
		kv = append(kv, keyValue{3, a.LastStirTime})
	}

	return kv
}

// ToOpenPrintTag converts Input (from API) to OpenPrintTag structure
func (i *Input) ToOpenPrintTag() (*OpenPrintTag, error) {
	opt := &OpenPrintTag{}

	// Set main section fields
	opt.Main.MaterialName = i.MaterialName
	opt.Main.BrandName = i.BrandName
	opt.Main.MaterialClass = MaterialClass(i.MaterialClass)
	opt.Main.MaterialType = MaterialType(i.MaterialType)
	opt.Main.NominalNettoFullWeight = i.NominalWeight
	opt.Main.FilamentDiameter = i.FilamentDiameter
	opt.Main.Density = i.Density
	opt.Main.MinPrintTemp = i.MinPrintTemp
	opt.Main.MaxPrintTemp = i.MaxPrintTemp
	opt.Main.ManufacturedDate = i.ManufacturedDate
	opt.Main.ExpirationDate = i.ExpirationDate

	// Parse and set UUIDs
	if i.InstanceUUID != "" {
		b, err := parseUUID(i.InstanceUUID)
		if err != nil {
			return nil, fmt.Errorf("invalid instanceUuid: %w", err)
		}
		opt.Main.InstanceUUID = b
	} else {
		// Generate a new UUID if not provided
		newUUID := uuid.New()
		opt.Main.InstanceUUID = newUUID[:]
	}

	if i.PackageUUID != "" {
		b, err := parseUUID(i.PackageUUID)
		if err != nil {
			return nil, fmt.Errorf("invalid packageUuid: %w", err)
		}
		opt.Main.PackageUUID = b
	}

	if i.MaterialUUID != "" {
		b, err := parseUUID(i.MaterialUUID)
		if err != nil {
			return nil, fmt.Errorf("invalid materialUuid: %w", err)
		}
		opt.Main.MaterialUUID = b
	} else {
		// Generate material UUID from brand + material name
		opt.Main.MaterialUUID = GenerateMaterialUUID(i.BrandName, i.MaterialName)
	}

	if i.BrandUUID != "" {
		b, err := parseUUID(i.BrandUUID)
		if err != nil {
			return nil, fmt.Errorf("invalid brandUuid: %w", err)
		}
		opt.Main.BrandUUID = b
	} else {
		// Generate brand UUID from brand name
		opt.Main.BrandUUID = GenerateBrandUUID(i.BrandName)
	}

	// Parse color
	if i.PrimaryColor != "" {
		c, err := parseHexColor(i.PrimaryColor)
		if err != nil {
			return nil, fmt.Errorf("invalid primaryColor: %w", err)
		}
		opt.Main.PrimaryColor = c
	}

	// Set auxiliary section fields
	opt.Aux.ConsumedWeight = i.ConsumedWeight
	opt.Aux.Workgroup = i.Workgroup

	return opt, nil
}

// Encode converts Input directly to CBOR bytes
func (i *Input) Encode() ([]byte, error) {
	opt, err := i.ToOpenPrintTag()
	if err != nil {
		return nil, err
	}
	return opt.Encode()
}

// OpenPrintTag namespace UUIDs for UUIDv5 generation (per spec section 3.2.1)
var (
	// Namespace for brand_uuid derivation
	brandNamespace = uuid.MustParse("5269dfb7-1559-440a-85be-aba5f3eff2d2")
	// Namespace for material_uuid derivation
	materialNamespace = uuid.MustParse("616fc86d-7d99-4953-96c7-46d2836b9be9")
	// Namespace for package_uuid derivation
	packageNamespace = uuid.MustParse("6f7d485e-db8d-4979-904e-a231cd6602b2")
	// Namespace for instance_uuid derivation
	instanceNamespace = uuid.MustParse("31062f81-b5bd-4f86-a5f8-46367e841508")
)

// GenerateBrandUUID creates a UUIDv5 for brand identification.
// Per OpenPrintTag spec: uuid5(brandNamespace, brand_name)
func GenerateBrandUUID(brandName string) []byte {
	u := uuid.NewSHA1(brandNamespace, []byte(brandName))
	return u[:]
}

// GenerateMaterialUUID creates a UUIDv5 for material identification.
// Per OpenPrintTag spec: uuid5(materialNamespace, brand_uuid + material_name)
func GenerateMaterialUUID(brandName, materialName string) []byte {
	// First get brand UUID
	brandUUID := GenerateBrandUUID(brandName)
	// Concatenate brand_uuid (binary) + material_name
	data := append(brandUUID, []byte(materialName)...)
	u := uuid.NewSHA1(materialNamespace, data)
	return u[:]
}

// GeneratePackageUUID creates a UUIDv5 for package identification.
// Per OpenPrintTag spec: uuid5(packageNamespace, brand_uuid + gtin)
func GeneratePackageUUID(brandUUID []byte, gtin string) []byte {
	// Concatenate brand_uuid (binary) + gtin (as string)
	data := append(brandUUID, []byte(gtin)...)
	u := uuid.NewSHA1(packageNamespace, data)
	return u[:]
}

// GenerateInstanceUUID creates a UUIDv5 for instance identification from NFC tag UID.
// Per OpenPrintTag spec: uuid5(instanceNamespace, nfc_tag_uid)
// The nfcTagUID should be 8 bytes with MSB first (e.g., 0xE0 as first byte for NFC-V).
func GenerateInstanceUUID(nfcTagUID []byte) []byte {
	u := uuid.NewSHA1(instanceNamespace, nfcTagUID)
	return u[:]
}
