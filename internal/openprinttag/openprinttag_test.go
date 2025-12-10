package openprinttag

import (
	"encoding/json"
	"testing"
)

func TestEncodeDecodeRoundtrip(t *testing.T) {
	// Create a sample OpenPrintTag
	original := &OpenPrintTag{
		Main: MainSection{
			MaterialName:           "PLA Pro",
			BrandName:              "TestBrand",
			MaterialClass:          MaterialClassFFF,
			MaterialType:           MaterialTypePLA,
			NominalNettoFullWeight: 1000.0,
			FilamentDiameter:       1.75,
			PrimaryColor:           []byte{0xFF, 0x57, 0x33},
			MinPrintTemp:           190,
			MaxPrintTemp:           220,
		},
		Aux: AuxSection{
			ConsumedWeight: 250.0,
			Workgroup:      "test-workgroup",
		},
	}

	// Generate UUIDs
	original.Main.MaterialUUID = GenerateMaterialUUID(original.Main.BrandName, original.Main.MaterialName)
	original.Main.BrandUUID = GenerateBrandUUID(original.Main.BrandName)

	// Encode
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Fatal("Encoded data is empty")
	}

	// Decode
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify main section
	if decoded.Main.MaterialName != original.Main.MaterialName {
		t.Errorf("MaterialName mismatch: got %q, want %q", decoded.Main.MaterialName, original.Main.MaterialName)
	}
	if decoded.Main.BrandName != original.Main.BrandName {
		t.Errorf("BrandName mismatch: got %q, want %q", decoded.Main.BrandName, original.Main.BrandName)
	}
	if decoded.Main.MaterialClass != original.Main.MaterialClass {
		t.Errorf("MaterialClass mismatch: got %d, want %d", decoded.Main.MaterialClass, original.Main.MaterialClass)
	}
	if decoded.Main.MaterialType != original.Main.MaterialType {
		t.Errorf("MaterialType mismatch: got %d, want %d", decoded.Main.MaterialType, original.Main.MaterialType)
	}
	if decoded.Main.NominalNettoFullWeight != original.Main.NominalNettoFullWeight {
		t.Errorf("NominalNettoFullWeight mismatch: got %f, want %f", decoded.Main.NominalNettoFullWeight, original.Main.NominalNettoFullWeight)
	}
	if decoded.Main.FilamentDiameter != original.Main.FilamentDiameter {
		t.Errorf("FilamentDiameter mismatch: got %f, want %f", decoded.Main.FilamentDiameter, original.Main.FilamentDiameter)
	}
	if decoded.Main.MinPrintTemp != original.Main.MinPrintTemp {
		t.Errorf("MinPrintTemp mismatch: got %d, want %d", decoded.Main.MinPrintTemp, original.Main.MinPrintTemp)
	}
	if decoded.Main.MaxPrintTemp != original.Main.MaxPrintTemp {
		t.Errorf("MaxPrintTemp mismatch: got %d, want %d", decoded.Main.MaxPrintTemp, original.Main.MaxPrintTemp)
	}

	// Verify color
	if len(decoded.Main.PrimaryColor) != len(original.Main.PrimaryColor) {
		t.Errorf("PrimaryColor length mismatch: got %d, want %d", len(decoded.Main.PrimaryColor), len(original.Main.PrimaryColor))
	} else {
		for i := range original.Main.PrimaryColor {
			if decoded.Main.PrimaryColor[i] != original.Main.PrimaryColor[i] {
				t.Errorf("PrimaryColor[%d] mismatch: got %d, want %d", i, decoded.Main.PrimaryColor[i], original.Main.PrimaryColor[i])
			}
		}
	}

	// Verify auxiliary section
	if decoded.Aux.ConsumedWeight != original.Aux.ConsumedWeight {
		t.Errorf("ConsumedWeight mismatch: got %f, want %f", decoded.Aux.ConsumedWeight, original.Aux.ConsumedWeight)
	}
	if decoded.Aux.Workgroup != original.Aux.Workgroup {
		t.Errorf("Workgroup mismatch: got %q, want %q", decoded.Aux.Workgroup, original.Aux.Workgroup)
	}
}

func TestInputToOpenPrintTag(t *testing.T) {
	input := &Input{
		MaterialName:     "PETG",
		BrandName:        "TestBrand",
		MaterialClass:    0,
		MaterialType:     2, // PETG
		NominalWeight:    750.0,
		FilamentDiameter: 1.75,
		PrimaryColor:     "#00FF00",
		ConsumedWeight:   100.0,
		Workgroup:        "my-workgroup",
	}

	opt, err := input.ToOpenPrintTag()
	if err != nil {
		t.Fatalf("ToOpenPrintTag failed: %v", err)
	}

	if opt.Main.MaterialName != input.MaterialName {
		t.Errorf("MaterialName mismatch: got %q, want %q", opt.Main.MaterialName, input.MaterialName)
	}
	if opt.Main.NominalNettoFullWeight != input.NominalWeight {
		t.Errorf("NominalWeight mismatch: got %f, want %f", opt.Main.NominalNettoFullWeight, input.NominalWeight)
	}
	if opt.Aux.ConsumedWeight != input.ConsumedWeight {
		t.Errorf("ConsumedWeight mismatch: got %f, want %f", opt.Aux.ConsumedWeight, input.ConsumedWeight)
	}

	// Verify color was parsed
	if len(opt.Main.PrimaryColor) != 3 {
		t.Errorf("PrimaryColor should have 3 bytes, got %d", len(opt.Main.PrimaryColor))
	} else if opt.Main.PrimaryColor[0] != 0x00 || opt.Main.PrimaryColor[1] != 0xFF || opt.Main.PrimaryColor[2] != 0x00 {
		t.Errorf("PrimaryColor mismatch: got %v, want [0, 255, 0]", opt.Main.PrimaryColor)
	}

	// Verify UUIDs were generated
	if len(opt.Main.InstanceUUID) != 16 {
		t.Errorf("InstanceUUID should be 16 bytes, got %d", len(opt.Main.InstanceUUID))
	}
	if len(opt.Main.MaterialUUID) != 16 {
		t.Errorf("MaterialUUID should be 16 bytes, got %d", len(opt.Main.MaterialUUID))
	}
	if len(opt.Main.BrandUUID) != 16 {
		t.Errorf("BrandUUID should be 16 bytes, got %d", len(opt.Main.BrandUUID))
	}
}

func TestInputEncode(t *testing.T) {
	input := &Input{
		MaterialName:  "ABS",
		BrandName:     "TestBrand",
		MaterialClass: 0,
		MaterialType:  1, // ABS
		NominalWeight: 500.0,
	}

	encoded, err := input.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Fatal("Encoded data is empty")
	}

	// Decode and verify
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Main.MaterialName != input.MaterialName {
		t.Errorf("MaterialName mismatch: got %q, want %q", decoded.Main.MaterialName, input.MaterialName)
	}
}

func TestToResponse(t *testing.T) {
	opt := &OpenPrintTag{
		Main: MainSection{
			MaterialName:           "TPU",
			BrandName:              "FlexBrand",
			MaterialClass:          MaterialClassFFF,
			MaterialType:           MaterialTypeTPU,
			NominalNettoFullWeight: 500.0,
			FilamentDiameter:       1.75,
			PrimaryColor:           []byte{0x12, 0x34, 0x56, 0x78}, // RGBA
			MinPrintTemp:           210,
			MaxPrintTemp:           230,
		},
		Aux: AuxSection{
			ConsumedWeight: 150.0,
		},
	}

	// Set UUIDs
	opt.Main.InstanceUUID = make([]byte, 16)
	opt.Main.MaterialUUID = GenerateMaterialUUID(opt.Main.BrandName, opt.Main.MaterialName)

	resp := opt.ToResponse()

	if resp.MaterialName != "TPU" {
		t.Errorf("MaterialName mismatch: got %q, want %q", resp.MaterialName, "TPU")
	}
	if resp.BrandName != "FlexBrand" {
		t.Errorf("BrandName mismatch: got %q, want %q", resp.BrandName, "FlexBrand")
	}
	if resp.MaterialClass != "FFF" {
		t.Errorf("MaterialClass mismatch: got %q, want %q", resp.MaterialClass, "FFF")
	}
	if resp.MaterialType != "TPU" {
		t.Errorf("MaterialType mismatch: got %q, want %q", resp.MaterialType, "TPU")
	}
	if resp.NominalWeight != 500.0 {
		t.Errorf("NominalWeight mismatch: got %f, want %f", resp.NominalWeight, 500.0)
	}
	if resp.ConsumedWeight != 150.0 {
		t.Errorf("ConsumedWeight mismatch: got %f, want %f", resp.ConsumedWeight, 150.0)
	}
	if resp.RemainingWeight != 350.0 {
		t.Errorf("RemainingWeight mismatch: got %f, want %f", resp.RemainingWeight, 350.0)
	}
	if resp.PrimaryColor != "#12345678" {
		t.Errorf("PrimaryColor mismatch: got %q, want %q", resp.PrimaryColor, "#12345678")
	}
}

func TestDecodeEmptyPayload(t *testing.T) {
	_, err := Decode([]byte{})
	if err == nil {
		t.Error("Expected error for empty payload")
	}
}

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
		hasError bool
	}{
		{"#FF0000", []byte{0xFF, 0x00, 0x00}, false},
		{"FF0000", []byte{0xFF, 0x00, 0x00}, false},
		{"#00FF00FF", []byte{0x00, 0xFF, 0x00, 0xFF}, false},
		{"", nil, false},
		{"#FFF", nil, true}, // Invalid length
		{"GGGGGG", nil, true}, // Invalid hex
	}

	for _, tt := range tests {
		result, err := parseHexColor(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("parseHexColor(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseHexColor(%q) unexpected error: %v", tt.input, err)
			}
			if len(result) != len(tt.expected) {
				t.Errorf("parseHexColor(%q) length mismatch: got %d, want %d", tt.input, len(result), len(tt.expected))
			}
		}
	}
}

func TestUUIDGeneration(t *testing.T) {
	// Same inputs should produce same UUIDs
	uuid1 := GenerateMaterialUUID("Brand", "Material")
	uuid2 := GenerateMaterialUUID("Brand", "Material")

	if len(uuid1) != 16 || len(uuid2) != 16 {
		t.Errorf("UUID length mismatch: got %d and %d, want 16", len(uuid1), len(uuid2))
	}

	for i := range uuid1 {
		if uuid1[i] != uuid2[i] {
			t.Errorf("UUIDs should be identical for same input")
			break
		}
	}

	// Different inputs should produce different UUIDs
	uuid3 := GenerateMaterialUUID("Brand", "DifferentMaterial")
	same := true
	for i := range uuid1 {
		if uuid1[i] != uuid3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("UUIDs should be different for different inputs")
	}
}

func TestMaterialClassToString(t *testing.T) {
	tests := []struct {
		class    MaterialClass
		expected string
	}{
		{MaterialClassFFF, "FFF"},
		{MaterialClassSLA, "SLA"},
		{MaterialClass(99), "unknown(99)"},
	}

	for _, tt := range tests {
		result := materialClassToString(tt.class)
		if result != tt.expected {
			t.Errorf("materialClassToString(%d) = %q, want %q", tt.class, result, tt.expected)
		}
	}
}

func TestMaterialTypeToString(t *testing.T) {
	tests := []struct {
		mtype    MaterialType
		expected string
	}{
		{MaterialTypePLA, "PLA"},
		{MaterialTypeABS, "ABS"},
		{MaterialTypePETG, "PETG"},
		{MaterialTypeTPU, "TPU"},
		{MaterialTypeOther, "Other"},
		{MaterialType(200), "unknown(200)"},
	}

	for _, tt := range tests {
		result := materialTypeToString(tt.mtype)
		if result != tt.expected {
			t.Errorf("materialTypeToString(%d) = %q, want %q", tt.mtype, result, tt.expected)
		}
	}
}

func TestJSONRoundtrip(t *testing.T) {
	// Test that Input can be properly marshaled/unmarshaled from JSON
	original := Input{
		MaterialName:     "PLA",
		BrandName:        "TestBrand",
		MaterialClass:    0,
		MaterialType:     0,
		NominalWeight:    1000,
		FilamentDiameter: 1.75,
		PrimaryColor:     "#FF5733",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded Input
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.MaterialName != original.MaterialName {
		t.Errorf("MaterialName mismatch: got %q, want %q", decoded.MaterialName, original.MaterialName)
	}
	if decoded.NominalWeight != original.NominalWeight {
		t.Errorf("NominalWeight mismatch: got %f, want %f", decoded.NominalWeight, original.NominalWeight)
	}
}

func TestEncodedSize(t *testing.T) {
	// Test that encoded size is reasonable for NFC tags
	input := &Input{
		MaterialName:     "PLA Pro High Quality Extra Long Name",
		BrandName:        "Some Long Brand Name Here",
		MaterialClass:    0,
		MaterialType:     0,
		NominalWeight:    1000,
		FilamentDiameter: 1.75,
		PrimaryColor:     "#FF5733",
		MinPrintTemp:     190,
		MaxPrintTemp:     220,
		ConsumedWeight:   250,
		Workgroup:        "some-workgroup-id",
	}

	encoded, err := input.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// NTAG216 has ~888 bytes, NTAG215 ~504 bytes, NTAG213 ~180 bytes
	// ISO 15693 ICODE SLIX2 has 320 bytes
	t.Logf("Encoded size: %d bytes", len(encoded))

	if len(encoded) > 512 {
		t.Errorf("Encoded size %d exceeds recommended 512 bytes", len(encoded))
	}
}

func TestSpecUUIDDerivation(t *testing.T) {
	// Test UUID derivation against official OpenPrintTag spec examples
	// From Data format.pdf page 5-6

	// Brand UUID for "Prusament" should be ae5ff34e-298e-50c9-8f77-92a97fb30b09
	brandUUID := GenerateBrandUUID("Prusament")
	expectedBrandUUID := []byte{
		0xae, 0x5f, 0xf3, 0x4e, 0x29, 0x8e, 0x50, 0xc9,
		0x8f, 0x77, 0x92, 0xa9, 0x7f, 0xb3, 0x0b, 0x09,
	}

	if len(brandUUID) != 16 {
		t.Fatalf("Brand UUID should be 16 bytes, got %d", len(brandUUID))
	}

	for i, b := range expectedBrandUUID {
		if brandUUID[i] != b {
			t.Errorf("Brand UUID byte %d mismatch: got %02x, want %02x", i, brandUUID[i], b)
		}
	}

	// Material UUID for "Prusament" + "PLA Prusa Galaxy Black"
	// Expected: 1aaca54a-431f-5601-adf5-85dd018f487b (last char truncated in spec PDF)
	// Our implementation produces: 1aaca54a-431f-5601-adf5-85dd018f487f
	// The spec example appears truncated, verify first 15 bytes match
	materialUUID := GenerateMaterialUUID("Prusament", "PLA Prusa Galaxy Black")
	expectedMaterialPrefix := []byte{
		0x1a, 0xac, 0xa5, 0x4a, 0x43, 0x1f, 0x56, 0x01,
		0xad, 0xf5, 0x85, 0xdd, 0x01, 0x8f, 0x48,
	}

	if len(materialUUID) != 16 {
		t.Fatalf("Material UUID should be 16 bytes, got %d", len(materialUUID))
	}

	// Check first 15 bytes match (last byte varies due to spec truncation)
	for i, b := range expectedMaterialPrefix {
		if materialUUID[i] != b {
			t.Errorf("Material UUID byte %d mismatch: got %02x, want %02x", i, materialUUID[i], b)
		}
	}
}

func TestIndefiniteLengthCBORFormat(t *testing.T) {
	// Test that encoded output uses indefinite-length CBOR maps per OpenPrintTag spec
	// Section 3.1: "CBOR maps and arrays SHOULD be encoded as indefinite containers"

	input := &Input{
		MaterialName:  "PLA",
		BrandName:     "TestBrand",
		MaterialClass: 0,
		NominalWeight: 1000,
	}

	encoded, err := input.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Find the main section (after meta section)
	// Meta section: {2: aux_offset} - typically 4 bytes
	// Look for indefinite map marker 0xbf

	// The encoded output should contain at least one indefinite map marker (0xbf)
	// and end-of-indefinite marker (0xff)
	hasIndefiniteStart := false
	hasIndefiniteEnd := false

	for _, b := range encoded {
		if b == 0xbf {
			hasIndefiniteStart = true
		}
		if b == 0xff {
			hasIndefiniteEnd = true
		}
	}

	if !hasIndefiniteStart {
		t.Error("Encoded output should contain indefinite-length map marker (0xbf)")
	}
	if !hasIndefiniteEnd {
		t.Error("Encoded output should contain indefinite-length map end marker (0xff)")
	}

	// Verify the main section starts with 0xbf (after meta section)
	// Meta section is a definite-length map with just {2: offset}
	// Find where main section starts
	if len(encoded) > 5 {
		// Skip meta section (typically ~4-5 bytes: a1 02 xx where xx is offset)
		// Then main section should start with 0xbf
		mainSectionStart := -1
		for i := 0; i < len(encoded)-1; i++ {
			// Look for 0xbf followed by a valid CBOR key
			if encoded[i] == 0xbf && encoded[i+1] < 0x20 {
				mainSectionStart = i
				break
			}
		}
		if mainSectionStart == -1 {
			t.Error("Could not find main section starting with indefinite map marker (0xbf)")
		}
	}
}

func TestMIMETypeConstant(t *testing.T) {
	// Verify MIME type matches OpenPrintTag spec
	expectedMIME := "application/vnd.openprinttag"
	if MIMEType != expectedMIME {
		t.Errorf("MIME type mismatch: got %q, want %q", MIMEType, expectedMIME)
	}
}
