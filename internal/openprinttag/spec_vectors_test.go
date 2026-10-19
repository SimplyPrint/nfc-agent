package openprinttag

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// specVectorPayload extracts the OpenPrintTag record payload from a spec test
// vector tag image: 4-byte ISO 15693 CC, then an NDEF TLV.
func specVectorPayload(t *testing.T, name string) []byte {
	t.Helper()
	img, err := os.ReadFile(filepath.Join("testdata", "spec", name))
	if err != nil {
		t.Fatal(err)
	}
	pos := 4
	if img[pos] != 0x03 {
		t.Fatalf("%s: expected NDEF TLV at offset 4, got 0x%02x", name, img[pos])
	}
	pos++
	msgLen := int(img[pos])
	pos++
	if msgLen == 0xFF {
		msgLen = int(img[pos])<<8 | int(img[pos+1])
		pos += 2
	}
	msg := img[pos : pos+msgLen]

	for p := 0; p < len(msg); {
		hdr := msg[p]
		typeLen := int(msg[p+1])
		p += 2
		var payloadLen int
		if hdr&0x10 != 0 { // short record
			payloadLen = int(msg[p])
			p++
		} else {
			payloadLen = int(msg[p])<<24 | int(msg[p+1])<<16 | int(msg[p+2])<<8 | int(msg[p+3])
			p += 4
		}
		idLen := 0
		if hdr&0x08 != 0 {
			idLen = int(msg[p])
			p++
		}
		recType := string(msg[p : p+typeLen])
		p += typeLen + idLen
		payload := msg[p : p+payloadLen]
		p += payloadLen
		if hdr&0x07 == 0x02 && recType == MIMEType {
			return payload
		}
	}
	t.Fatalf("%s: no %s record found", name, MIMEType)
	return nil
}

// TestSpecVectors decodes the official OpenPrintTag test vectors and checks
// material types against the spec enum and UUIDs against the spec's expected
// derivations (issue #7: Prusament tags don't store UUIDs on the tag).
func TestSpecVectors(t *testing.T) {
	tests := []struct {
		file         string
		tagUID       string // MSB-first, from the vector's test_config
		materialType string
		materialID   *int
		brand        string
		material     string
		pkg          string
		instance     string
	}{
		{
			file: "01_data.bin", tagUID: "E0040108662F6FBC",
			materialType: "PLA", materialID: intPtr(0),
			brand:    "ae5ff34e-298e-50c9-8f77-92a97fb30b09",
			material: "1aaca54a-431f-5601-adf5-85dd018f487f",
			pkg:      "6e0aece2-1daf-5f2a-ba20-697968ec7d14",
			instance: "bf63e92d-9ca5-53d7-9fab-ffdd0240c585",
		},
		{
			file:         "02_data.bin",
			materialType: "PETG", materialID: intPtr(1),
			brand:    "ae5ff34e-298e-50c9-8f77-92a97fb30b09",
			material: "1378e978-35ed-534c-9dfa-a65525bf8649",
			pkg:      "6f957b59-9725-5068-9102-15bb77807534",
		},
		{file: "03_data.bin", materialType: "PETG", materialID: intPtr(1)}, // no brand_name: nothing to derive
		{file: "04_data.bin", materialType: "PEKK", materialID: intPtr(21)},
		{file: "05_data.bin"}, // unknown tag value 9429, no material_type
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			opt, err := Decode(specVectorPayload(t, tt.file))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			var uid []byte
			if tt.tagUID != "" {
				uid, _ = hex.DecodeString(tt.tagUID)
			}
			resp := opt.ToResponseForTag(uid)

			if resp.MaterialType != tt.materialType {
				t.Errorf("materialType = %q, want %q", resp.MaterialType, tt.materialType)
			}
			if !reflect.DeepEqual(resp.MaterialTypeID, tt.materialID) {
				t.Errorf("materialTypeId = %v, want %v", deref(resp.MaterialTypeID), deref(tt.materialID))
			}
			if tt.brand != "" || tt.material != "" || tt.pkg != "" || tt.instance != "" {
				if resp.BrandUUID != tt.brand {
					t.Errorf("brandUuid = %q, want %q", resp.BrandUUID, tt.brand)
				}
				if resp.MaterialUUID != tt.material {
					t.Errorf("materialUuid = %q, want %q", resp.MaterialUUID, tt.material)
				}
				if resp.PackageUUID != tt.pkg {
					t.Errorf("packageUuid = %q, want %q", resp.PackageUUID, tt.pkg)
				}
				if resp.InstanceUUID != tt.instance {
					t.Errorf("instanceUuid = %q, want %q", resp.InstanceUUID, tt.instance)
				}
			}
		})
	}
}

// TestToResponseForTagKeepsStoredUUIDs verifies UUIDs stored on the tag win
// over derived ones, and that derived material/package UUIDs use the stored
// brand_uuid.
func TestToResponseForTagKeepsStoredUUIDs(t *testing.T) {
	stored := []byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}
	opt := &OpenPrintTag{Main: MainSection{
		BrandName: "Prusament", MaterialName: "PETG Jet Black", GTIN: 8594173675100,
		BrandUUID: stored, InstanceUUID: stored,
	}}
	resp := opt.ToResponseForTag([]byte{0xE0, 0x04, 0x01, 0x08, 0x66, 0x2F, 0x6F, 0xBC})

	want := formatUUID(stored)
	if resp.BrandUUID != want || resp.InstanceUUID != want {
		t.Errorf("stored UUIDs not kept: brand=%s instance=%s", resp.BrandUUID, resp.InstanceUUID)
	}
	if got, w := resp.MaterialUUID, formatUUID(generateMaterialUUIDFromBrand(stored, "PETG Jet Black")); got != w {
		t.Errorf("materialUuid = %s, want derived from stored brand %s", got, w)
	}
	if got, w := resp.PackageUUID, formatUUID(GeneratePackageUUID(stored, "8594173675100")); got != w {
		t.Errorf("packageUuid = %s, want derived from stored brand %s", got, w)
	}
}

// TestInputMaterialTypeCompat verifies the write API keeps the meaning of the
// pre-v0.8 integer materialType values while writing spec-correct values, and
// accepts names and raw spec ids.
func TestInputMaterialTypeCompat(t *testing.T) {
	tests := []struct {
		name     string
		input    string // JSON fragment merged into a minimal Input
		wantType *int
		wantTags []uint16
		wantErr  bool
	}{
		{name: "omitted defaults to PLA", input: `{}`, wantType: intPtr(0)},
		{name: "legacy 0 PLA", input: `{"materialType":0}`, wantType: intPtr(0)},
		{name: "legacy 1 ABS", input: `{"materialType":1}`, wantType: intPtr(3)},
		{name: "legacy 2 PETG", input: `{"materialType":2}`, wantType: intPtr(1)},
		{name: "legacy 3 ASA", input: `{"materialType":3}`, wantType: intPtr(4)},
		{name: "legacy 4 PC", input: `{"materialType":4}`, wantType: intPtr(5)},
		{name: "legacy 5 Nylon", input: `{"materialType":5}`, wantType: intPtr(10)},
		{name: "legacy 6 TPU", input: `{"materialType":6}`, wantType: intPtr(2)},
		{name: "legacy 7 PVA", input: `{"materialType":7}`, wantType: intPtr(20)},
		{name: "legacy 8 HIPS", input: `{"materialType":8}`, wantType: intPtr(14)},
		{name: "legacy 9 PP", input: `{"materialType":9}`, wantType: intPtr(7)},
		{name: "legacy 10 PEI", input: `{"materialType":10}`, wantType: intPtr(17)},
		{name: "legacy 11 PEEK", input: `{"materialType":11}`, wantType: intPtr(22)},
		{name: "legacy 12 PA", input: `{"materialType":12}`, wantType: intPtr(10)},
		{name: "legacy 13 PA-CF", input: `{"materialType":13}`, wantType: intPtr(10), wantTags: []uint16{31, 4}},
		{name: "legacy 14 PA-GF", input: `{"materialType":14}`, wantType: intPtr(10), wantTags: []uint16{34, 4}},
		{name: "legacy 15 PLA-CF", input: `{"materialType":15}`, wantType: intPtr(0), wantTags: []uint16{31, 4}},
		{name: "legacy 16 PLA-GF", input: `{"materialType":16}`, wantType: intPtr(0), wantTags: []uint16{34, 4}},
		{name: "legacy 17 PETG-CF", input: `{"materialType":17}`, wantType: intPtr(1), wantTags: []uint16{31, 4}},
		{name: "legacy 18 PETG-GF", input: `{"materialType":18}`, wantType: intPtr(1), wantTags: []uint16{34, 4}},
		{name: "legacy 255 Other omits type", input: `{"materialType":255}`},
		{name: "legacy undefined int written as-is", input: `{"materialType":40}`, wantType: intPtr(40)},
		{name: "legacy int out of range", input: `{"materialType":256}`, wantErr: true},
		{name: "spec name", input: `{"materialType":"PETG"}`, wantType: intPtr(1)},
		{name: "spec name case-insensitive", input: `{"materialType":"pa612"}`, wantType: intPtr(42)},
		{name: "composite alias", input: `{"materialType":"PLA-CF"}`, wantType: intPtr(0), wantTags: []uint16{31, 4}},
		{name: "alias Other omits type", input: `{"materialType":"Other"}`},
		{name: "unknown name", input: `{"materialType":"Unobtainium"}`, wantErr: true},
		{name: "spec id overrides", input: `{"materialType":2,"materialTypeId":21}`, wantType: intPtr(21)},
		{name: "spec id out of range", input: `{"materialTypeId":70000}`, wantErr: true},
		{name: "SLA without type omits it", input: `{"materialClass":1}`},
		{name: "extra tags merged without duplicates", input: `{"materialType":"PETG-CF","tags":[4,5]}`, wantType: intPtr(1), wantTags: []uint16{31, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{MaterialName: "Test", BrandName: "Brand", NominalWeight: 1000}
			if err := json.Unmarshal([]byte(tt.input), &input); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			opt, err := input.ToOpenPrintTag()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToOpenPrintTag: %v", err)
			}

			// Round-trip through CBOR to check what actually lands on the tag.
			payload, err := opt.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			decoded, err := Decode(payload)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			var gotType *int
			if decoded.Main.MaterialType != nil {
				v := int(*decoded.Main.MaterialType)
				gotType = &v
			}
			if !reflect.DeepEqual(gotType, tt.wantType) {
				t.Errorf("material_type on tag = %v, want %v", deref(gotType), deref(tt.wantType))
			}
			if len(decoded.Main.Tags) != 0 || len(tt.wantTags) != 0 {
				if !reflect.DeepEqual(decoded.Main.Tags, tt.wantTags) {
					t.Errorf("tags on tag = %v, want %v", decoded.Main.Tags, tt.wantTags)
				}
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func mtPtr(v MaterialType) *MaterialType { return &v }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// TestLegacyOtherDecodes verifies tags written by pre-v0.8 agents with
// materialType 255 ("Other") still read as "Other".
func TestLegacyOtherDecodes(t *testing.T) {
	if got := materialTypeToString(255); got != "Other" {
		t.Errorf("materialTypeToString(255) = %q, want Other", got)
	}
	if got := materialTypeToString(200); got != "unknown(200)" {
		t.Errorf("materialTypeToString(200) = %q, want unknown(200)", got)
	}
}

// TestInputValidate verifies bad material types are caught before writing.
func TestInputValidate(t *testing.T) {
	for _, raw := range []string{`{"materialType":"Foo"}`, `{"materialType":-1}`, `{"materialType":true}`, `{"materialTypeId":-5}`} {
		var in Input
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatal(err)
		}
		if in.Validate() == nil {
			t.Errorf("Validate(%s) = nil, want error", raw)
		}
	}
	var ok Input
	_ = json.Unmarshal([]byte(`{"materialType":2}`), &ok)
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate(legacy 2) = %v", err)
	}
}

// TestMaterialClassInput verifies materialClass accepts the read format
// ("FFF"/"SLA") as well as numbers (issue #13).
func TestMaterialClassInput(t *testing.T) {
	tests := []struct {
		in      string
		want    MaterialClassInput
		wantErr bool
	}{
		{`{"materialClass":0}`, 0, false},
		{`{"materialClass":1}`, 1, false},
		{`{"materialClass":"FFF"}`, 0, false},
		{`{"materialClass":"sla"}`, 1, false},
		{`{}`, 0, false},
		{`{"materialClass":"resin"}`, 0, true},
		{`{"materialClass":-1}`, 0, true},
	}
	for _, tt := range tests {
		var in Input
		err := json.Unmarshal([]byte(tt.in), &in)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && in.MaterialClass != tt.want {
			t.Errorf("%s: materialClass = %d, want %d", tt.in, in.MaterialClass, tt.want)
		}
	}
}

// TestReadResponseRoundTrip verifies a read response can be sent back as a
// write unchanged (issue #13): same material, and an instance UUID that was
// only derived is not suddenly stored.
func TestReadResponseRoundTrip(t *testing.T) {
	opt, err := Decode(specVectorPayload(t, "02_data.bin"))
	if err != nil {
		t.Fatal(err)
	}
	uid := []byte{0xE0, 0x04, 0x01, 0x08, 0x66, 0x2F, 0x6F, 0xBC}
	readJSON, err := json.Marshal(opt.ToResponseForTag(uid))
	if err != nil {
		t.Fatal(err)
	}

	var in Input
	if err := json.Unmarshal(readJSON, &in); err != nil {
		t.Fatalf("read response rejected as write input: %v", err)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Writers (core.WriteDataWithURL) drop an instance UUID equal to the derived one.
	if in.InstanceUUID != DerivedInstanceUUID(uid) {
		t.Fatalf("instanceUuid = %s, want derived %s", in.InstanceUUID, DerivedInstanceUUID(uid))
	}
	in.InstanceUUID = ""
	written, err := in.ToOpenPrintTag()
	if err != nil {
		t.Fatal(err)
	}
	if written.Main.MaterialType == nil || *written.Main.MaterialType != MaterialTypePETG {
		t.Errorf("material type changed on round trip: %v", written.Main.MaterialType)
	}
	if len(written.Main.InstanceUUID) != 0 {
		t.Error("derived instance UUID should not be stored")
	}
	if got := written.ToResponseForTag(uid).InstanceUUID; got != DerivedInstanceUUID(uid) {
		t.Errorf("instance UUID after rewrite = %s, want unchanged %s", got, DerivedInstanceUUID(uid))
	}
}
