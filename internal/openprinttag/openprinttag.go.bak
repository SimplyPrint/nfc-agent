// Package openprinttag implements the OpenPrintTag NFC data format specification.
// See https://specs.openprinttag.org for the full specification.
package openprinttag

import (
	"encoding/hex"
	"fmt"
)

// MIME type for OpenPrintTag NDEF records (per OpenPrintTag spec)
const MIMEType = "application/vnd.openprinttag"

// MaxSectionSize is the maximum size for any section (512 bytes per spec)
const MaxSectionSize = 512

// MaterialClass enum values per OpenPrintTag spec
type MaterialClass uint8

const (
	MaterialClassFFF MaterialClass = 0 // Fused Filament Fabrication
	MaterialClassSLA MaterialClass = 1 // Stereolithography (resin)
)

// MaterialType enum values for FFF (MaterialClass 0)
type MaterialType uint8

const (
	// FFF material types
	MaterialTypePLA      MaterialType = 0
	MaterialTypeABS      MaterialType = 1
	MaterialTypePETG     MaterialType = 2
	MaterialTypeASA      MaterialType = 3
	MaterialTypePC       MaterialType = 4
	MaterialTypeNylon    MaterialType = 5
	MaterialTypeTPU      MaterialType = 6
	MaterialTypePVA      MaterialType = 7
	MaterialTypeHIPS     MaterialType = 8
	MaterialTypePP       MaterialType = 9
	MaterialTypePEI      MaterialType = 10
	MaterialTypePEEK     MaterialType = 11
	MaterialTypePA       MaterialType = 12
	MaterialTypePACF     MaterialType = 13
	MaterialTypePAGF     MaterialType = 14
	MaterialTypePLACF    MaterialType = 15
	MaterialTypePLAGF    MaterialType = 16
	MaterialTypePETGCF   MaterialType = 17
	MaterialTypePETGGF   MaterialType = 18
	MaterialTypeOther    MaterialType = 255
)

// MetaSection contains offsets and sizes for other sections.
// Keys 0-3 as per OpenPrintTag spec.
type MetaSection struct {
	MainOffset uint16 `cbor:"0,keyasint,omitempty"`
	MainSize   uint16 `cbor:"1,keyasint,omitempty"`
	AuxOffset  uint16 `cbor:"2,keyasint,omitempty"`
	AuxSize    uint16 `cbor:"3,keyasint,omitempty"`
}

// MainSection contains immutable material properties.
// Keys 0-56 as per OpenPrintTag spec.
type MainSection struct {
	// UUIDs (keys 0-3) - stored as raw bytes (16 bytes each)
	InstanceUUID []byte `cbor:"0,keyasint,omitempty"`
	PackageUUID  []byte `cbor:"1,keyasint,omitempty"`
	MaterialUUID []byte `cbor:"2,keyasint,omitempty"`
	BrandUUID    []byte `cbor:"3,keyasint,omitempty"`

	// Identifiers (keys 4-7)
	GTIN                     uint64 `cbor:"4,keyasint,omitempty"`
	BrandSpecificInstanceID  string `cbor:"5,keyasint,omitempty"`
	BrandSpecificPackageID   string `cbor:"6,keyasint,omitempty"`
	BrandSpecificMaterialID  string `cbor:"7,keyasint,omitempty"`

	// Material classification (keys 8-11)
	MaterialClass MaterialClass `cbor:"8,keyasint,omitempty"`
	MaterialType  MaterialType  `cbor:"9,keyasint,omitempty"`
	MaterialName  string        `cbor:"10,keyasint,omitempty"`
	BrandName     string        `cbor:"11,keyasint,omitempty"`

	// Protection and dates (keys 13-15)
	WriteProtection  uint8  `cbor:"13,keyasint,omitempty"`
	ManufacturedDate uint32 `cbor:"14,keyasint,omitempty"` // Unix timestamp
	ExpirationDate   uint32 `cbor:"15,keyasint,omitempty"` // Unix timestamp

	// Weight data (keys 16-18)
	NominalNettoFullWeight float32 `cbor:"16,keyasint,omitempty"`
	ActualNettoFullWeight  float32 `cbor:"17,keyasint,omitempty"`
	EmptyContainerWeight   float32 `cbor:"18,keyasint,omitempty"`

	// Colors (keys 19-24) - RGB or RGBA as byte arrays
	PrimaryColor    []byte `cbor:"19,keyasint,omitempty"`
	SecondaryColor0 []byte `cbor:"20,keyasint,omitempty"`
	SecondaryColor1 []byte `cbor:"21,keyasint,omitempty"`
	SecondaryColor2 []byte `cbor:"22,keyasint,omitempty"`
	SecondaryColor3 []byte `cbor:"23,keyasint,omitempty"`
	SecondaryColor4 []byte `cbor:"24,keyasint,omitempty"`

	// Transmission distance (key 27) - HueForge TD value
	TransmissionDistance float32 `cbor:"27,keyasint,omitempty"`

	// Tags (key 28)
	Tags []uint8 `cbor:"28,keyasint,omitempty"`

	// Physical properties (keys 29-33)
	Density           float32 `cbor:"29,keyasint,omitempty"`
	FilamentDiameter  float32 `cbor:"30,keyasint,omitempty"`
	ShoreHardnessA    uint8   `cbor:"31,keyasint,omitempty"`
	ShoreHardnessD    uint8   `cbor:"32,keyasint,omitempty"`
	MinNozzleDiameter float32 `cbor:"33,keyasint,omitempty"`

	// Temperature settings (keys 34-41)
	MinPrintTemp   uint16 `cbor:"34,keyasint,omitempty"`
	MaxPrintTemp   uint16 `cbor:"35,keyasint,omitempty"`
	PreheatTemp    uint16 `cbor:"36,keyasint,omitempty"`
	MinBedTemp     uint16 `cbor:"37,keyasint,omitempty"`
	MaxBedTemp     uint16 `cbor:"38,keyasint,omitempty"`
	MinChamberTemp uint16 `cbor:"39,keyasint,omitempty"`
	MaxChamberTemp uint16 `cbor:"40,keyasint,omitempty"`
	ChamberTemp    uint16 `cbor:"41,keyasint,omitempty"`

	// Container/spool dimensions (keys 42-45)
	ContainerWidth         uint16 `cbor:"42,keyasint,omitempty"` // mm
	ContainerOuterDiameter uint16 `cbor:"43,keyasint,omitempty"` // mm
	ContainerInnerDiameter uint16 `cbor:"44,keyasint,omitempty"` // mm
	ContainerHoleDiameter  uint16 `cbor:"45,keyasint,omitempty"` // mm

	// SLA viscosity (keys 46-49) - mPa·s
	Viscosity18C float32 `cbor:"46,keyasint,omitempty"`
	Viscosity25C float32 `cbor:"47,keyasint,omitempty"`
	Viscosity40C float32 `cbor:"48,keyasint,omitempty"`
	Viscosity60C float32 `cbor:"49,keyasint,omitempty"`

	// SLA container/curing (keys 50-51)
	ContainerVolumetricCapacity float32 `cbor:"50,keyasint,omitempty"` // ml (cm³)
	CureWavelength              uint16  `cbor:"51,keyasint,omitempty"` // nm

	// Additional metadata (keys 52-56)
	MaterialAbbreviation string  `cbor:"52,keyasint,omitempty"`
	NominalFullLength    uint32  `cbor:"53,keyasint,omitempty"` // mm
	ActualFullLength     uint32  `cbor:"54,keyasint,omitempty"` // mm
	CountryOfOrigin      string  `cbor:"55,keyasint,omitempty"`
	Certifications       []uint8 `cbor:"56,keyasint,omitempty"`
}

// AuxSection contains mutable runtime data that printers can update.
// Keys 0-3 as per OpenPrintTag spec.
type AuxSection struct {
	ConsumedWeight       float32 `cbor:"0,keyasint,omitempty"`
	Workgroup            string  `cbor:"1,keyasint,omitempty"`
	GeneralPurposeUser   string  `cbor:"2,keyasint,omitempty"`
	LastStirTime         uint32  `cbor:"3,keyasint,omitempty"` // Unix timestamp (for resin)
}

// OpenPrintTag represents the complete data structure
type OpenPrintTag struct {
	Meta MetaSection
	Main MainSection
	Aux  AuxSection
}

// Response is the JSON-friendly API response structure
type Response struct {
	// Material identification
	MaterialName  string `json:"materialName,omitempty"`
	BrandName     string `json:"brandName,omitempty"`
	MaterialClass string `json:"materialClass,omitempty"`
	MaterialType  string `json:"materialType,omitempty"`

	// UUIDs as strings
	InstanceUUID  string `json:"instanceUuid,omitempty"`
	PackageUUID   string `json:"packageUuid,omitempty"`
	MaterialUUID  string `json:"materialUuid,omitempty"`
	BrandUUID     string `json:"brandUuid,omitempty"`

	// Weight information
	NominalWeight   float32 `json:"nominalWeight,omitempty"`
	ConsumedWeight  float32 `json:"consumedWeight,omitempty"`
	RemainingWeight float32 `json:"remainingWeight,omitempty"`

	// Physical properties
	PrimaryColor     string  `json:"primaryColor,omitempty"` // hex #RRGGBB or #RRGGBBAA
	FilamentDiameter float32 `json:"filamentDiameter,omitempty"`
	FilamentLength   uint32  `json:"filamentLength,omitempty"` // in mm
	Density          float32 `json:"density,omitempty"`

	// Weight details
	ActualWeight float32 `json:"actualWeight,omitempty"` // actual netto weight
	SpoolWeight  float32 `json:"spoolWeight,omitempty"`  // empty container weight

	// Temperature settings
	MinPrintTemp uint16 `json:"minPrintTemp,omitempty"`
	MaxPrintTemp uint16 `json:"maxPrintTemp,omitempty"`
	MinBedTemp   uint16 `json:"minBedTemp,omitempty"`
	MaxBedTemp   uint16 `json:"maxBedTemp,omitempty"`

	// Dates
	ManufacturedDate uint32 `json:"manufacturedDate,omitempty"`
	ExpirationDate   uint32 `json:"expirationDate,omitempty"`

	// Auxiliary data
	Workgroup string `json:"workgroup,omitempty"`
}

// Input is the JSON structure for API write requests
type Input struct {
	// Required fields
	MaterialName  string `json:"materialName"`
	BrandName     string `json:"brandName"`
	MaterialClass int    `json:"materialClass"`
	MaterialType  int    `json:"materialType"`
	NominalWeight float32 `json:"nominalWeight"`

	// Optional fields
	InstanceUUID     string  `json:"instanceUuid,omitempty"`
	PackageUUID      string  `json:"packageUuid,omitempty"`
	MaterialUUID     string  `json:"materialUuid,omitempty"`
	BrandUUID        string  `json:"brandUuid,omitempty"`
	FilamentDiameter float32 `json:"filamentDiameter,omitempty"`
	PrimaryColor     string  `json:"primaryColor,omitempty"` // hex #RRGGBB or #RRGGBBAA
	Density          float32 `json:"density,omitempty"`
	MinPrintTemp     uint16  `json:"minPrintTemp,omitempty"`
	MaxPrintTemp     uint16  `json:"maxPrintTemp,omitempty"`
	ConsumedWeight   float32 `json:"consumedWeight,omitempty"`
	Workgroup        string  `json:"workgroup,omitempty"`
	ManufacturedDate uint32  `json:"manufacturedDate,omitempty"`
	ExpirationDate   uint32  `json:"expirationDate,omitempty"`
}

// ToResponse converts internal OpenPrintTag to API response
func (o *OpenPrintTag) ToResponse() *Response {
	resp := &Response{
		MaterialName:     o.Main.MaterialName,
		BrandName:        o.Main.BrandName,
		MaterialClass:    materialClassToString(o.Main.MaterialClass),
		MaterialType:     materialTypeToString(o.Main.MaterialType),
		NominalWeight:    o.Main.NominalNettoFullWeight,
		ActualWeight:     o.Main.ActualNettoFullWeight,
		SpoolWeight:      o.Main.EmptyContainerWeight,
		ConsumedWeight:   o.Aux.ConsumedWeight,
		FilamentDiameter: o.Main.FilamentDiameter,
		FilamentLength:   o.Main.ActualFullLength,
		Density:          o.Main.Density,
		MinPrintTemp:     o.Main.MinPrintTemp,
		MaxPrintTemp:     o.Main.MaxPrintTemp,
		MinBedTemp:       o.Main.MinBedTemp,
		MaxBedTemp:       o.Main.MaxBedTemp,
		ManufacturedDate: o.Main.ManufacturedDate,
		ExpirationDate:   o.Main.ExpirationDate,
		Workgroup:        o.Aux.Workgroup,
	}

	// Calculate remaining weight
	if o.Main.NominalNettoFullWeight > 0 {
		resp.RemainingWeight = o.Main.NominalNettoFullWeight - o.Aux.ConsumedWeight
		if resp.RemainingWeight < 0 {
			resp.RemainingWeight = 0
		}
	}

	// Convert UUIDs to strings
	if len(o.Main.InstanceUUID) == 16 {
		resp.InstanceUUID = formatUUID(o.Main.InstanceUUID)
	}
	if len(o.Main.PackageUUID) == 16 {
		resp.PackageUUID = formatUUID(o.Main.PackageUUID)
	}
	if len(o.Main.MaterialUUID) == 16 {
		resp.MaterialUUID = formatUUID(o.Main.MaterialUUID)
	}
	if len(o.Main.BrandUUID) == 16 {
		resp.BrandUUID = formatUUID(o.Main.BrandUUID)
	}

	// Convert color to hex string
	if len(o.Main.PrimaryColor) >= 3 {
		resp.PrimaryColor = colorToHex(o.Main.PrimaryColor)
	}

	return resp
}

// materialClassToString converts MaterialClass enum to string
func materialClassToString(mc MaterialClass) string {
	switch mc {
	case MaterialClassFFF:
		return "FFF"
	case MaterialClassSLA:
		return "SLA"
	default:
		return fmt.Sprintf("unknown(%d)", mc)
	}
}

// materialTypeToString converts MaterialType enum to string
func materialTypeToString(mt MaterialType) string {
	names := map[MaterialType]string{
		MaterialTypePLA:    "PLA",
		MaterialTypeABS:    "ABS",
		MaterialTypePETG:   "PETG",
		MaterialTypeASA:    "ASA",
		MaterialTypePC:     "PC",
		MaterialTypeNylon:  "Nylon",
		MaterialTypeTPU:    "TPU",
		MaterialTypePVA:    "PVA",
		MaterialTypeHIPS:   "HIPS",
		MaterialTypePP:     "PP",
		MaterialTypePEI:    "PEI",
		MaterialTypePEEK:   "PEEK",
		MaterialTypePA:     "PA",
		MaterialTypePACF:   "PA-CF",
		MaterialTypePAGF:   "PA-GF",
		MaterialTypePLACF:  "PLA-CF",
		MaterialTypePLAGF:  "PLA-GF",
		MaterialTypePETGCF: "PETG-CF",
		MaterialTypePETGGF: "PETG-GF",
		MaterialTypeOther:  "Other",
	}
	if name, ok := names[mt]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", mt)
}

// formatUUID converts 16 bytes to UUID string format
func formatUUID(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// colorToHex converts RGB/RGBA bytes to hex string
func colorToHex(c []byte) string {
	if len(c) == 3 {
		return fmt.Sprintf("#%02X%02X%02X", c[0], c[1], c[2])
	} else if len(c) == 4 {
		return fmt.Sprintf("#%02X%02X%02X%02X", c[0], c[1], c[2], c[3])
	}
	return ""
}

// parseHexColor parses #RRGGBB or #RRGGBBAA to byte slice
func parseHexColor(s string) ([]byte, error) {
	if len(s) == 0 {
		return nil, nil
	}
	if s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 && len(s) != 8 {
		return nil, fmt.Errorf("invalid color format: %s", s)
	}
	return hex.DecodeString(s)
}

// parseUUID parses UUID string to 16 bytes
func parseUUID(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	// Remove dashes
	clean := ""
	for _, c := range s {
		if c != '-' {
			clean += string(c)
		}
	}
	if len(clean) != 32 {
		return nil, fmt.Errorf("invalid UUID format: %s", s)
	}
	return hex.DecodeString(clean)
}
