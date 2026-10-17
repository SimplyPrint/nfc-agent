// Package openprinttag implements the OpenPrintTag NFC data format specification.
// See https://specs.openprinttag.org for the full specification.
package openprinttag

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MIME type for OpenPrintTag NDEF records (per OpenPrintTag spec)
const MIMEType = "application/vnd.openprinttag"

// MaxSectionSize is the maximum size for any section (512 bytes per spec)
const MaxSectionSize = 512

// MaterialClass enum values per OpenPrintTag spec.
// uint16 so tags with enum values added by future spec versions still decode.
type MaterialClass uint16

const (
	MaterialClassFFF MaterialClass = 0 // Fused Filament Fabrication
	MaterialClassSLA MaterialClass = 1 // Stereolithography (resin)
)

// MaterialType enum values for FFF (MaterialClass 0), per the OpenPrintTag
// spec's data/material_type_enum.yaml. uint16 so tags with enum values added
// by future spec versions still decode.
type MaterialType uint16

const (
	MaterialTypePLA   MaterialType = 0
	MaterialTypePETG  MaterialType = 1
	MaterialTypeTPU   MaterialType = 2
	MaterialTypeABS   MaterialType = 3
	MaterialTypeASA   MaterialType = 4
	MaterialTypePC    MaterialType = 5
	MaterialTypePCTG  MaterialType = 6
	MaterialTypePP    MaterialType = 7
	MaterialTypePA6   MaterialType = 8
	MaterialTypePA11  MaterialType = 9
	MaterialTypePA12  MaterialType = 10
	MaterialTypePA66  MaterialType = 11
	MaterialTypeCPE   MaterialType = 12
	MaterialTypeTPE   MaterialType = 13
	MaterialTypeHIPS  MaterialType = 14
	MaterialTypePHA   MaterialType = 15
	MaterialTypePET   MaterialType = 16
	MaterialTypePEI   MaterialType = 17
	MaterialTypePBT   MaterialType = 18
	MaterialTypePVB   MaterialType = 19
	MaterialTypePVA   MaterialType = 20
	MaterialTypePEKK  MaterialType = 21
	MaterialTypePEEK  MaterialType = 22
	MaterialTypeBVOH  MaterialType = 23
	MaterialTypeTPC   MaterialType = 24
	MaterialTypePPS   MaterialType = 25
	MaterialTypePPSU  MaterialType = 26
	MaterialTypePVC   MaterialType = 27
	MaterialTypePEBA  MaterialType = 28
	MaterialTypePVDF  MaterialType = 29
	MaterialTypePPA   MaterialType = 30
	MaterialTypePCL   MaterialType = 31
	MaterialTypePES   MaterialType = 32
	MaterialTypePMMA  MaterialType = 33
	MaterialTypePOM   MaterialType = 34
	MaterialTypePPE   MaterialType = 35
	MaterialTypePS    MaterialType = 36
	MaterialTypePSU   MaterialType = 37
	MaterialTypeTPI   MaterialType = 38
	MaterialTypeSBS   MaterialType = 39
	MaterialTypeOBC   MaterialType = 40
	MaterialTypeEVA   MaterialType = 41
	MaterialTypePA612 MaterialType = 42
)

// Tag enum values (data/tags_enum.yaml) used for composite materials.
const (
	TagAbrasive            uint16 = 4
	TagContainsCarbonFiber uint16 = 31
	TagContainsGlassFiber  uint16 = 34
)

// materialTypeNames maps spec material_type values to their abbreviations.
var materialTypeNames = map[MaterialType]string{
	MaterialTypePLA: "PLA", MaterialTypePETG: "PETG", MaterialTypeTPU: "TPU",
	MaterialTypeABS: "ABS", MaterialTypeASA: "ASA", MaterialTypePC: "PC",
	MaterialTypePCTG: "PCTG", MaterialTypePP: "PP", MaterialTypePA6: "PA6",
	MaterialTypePA11: "PA11", MaterialTypePA12: "PA12", MaterialTypePA66: "PA66",
	MaterialTypeCPE: "CPE", MaterialTypeTPE: "TPE", MaterialTypeHIPS: "HIPS",
	MaterialTypePHA: "PHA", MaterialTypePET: "PET", MaterialTypePEI: "PEI",
	MaterialTypePBT: "PBT", MaterialTypePVB: "PVB", MaterialTypePVA: "PVA",
	MaterialTypePEKK: "PEKK", MaterialTypePEEK: "PEEK", MaterialTypeBVOH: "BVOH",
	MaterialTypeTPC: "TPC", MaterialTypePPS: "PPS", MaterialTypePPSU: "PPSU",
	MaterialTypePVC: "PVC", MaterialTypePEBA: "PEBA", MaterialTypePVDF: "PVDF",
	MaterialTypePPA: "PPA", MaterialTypePCL: "PCL", MaterialTypePES: "PES",
	MaterialTypePMMA: "PMMA", MaterialTypePOM: "POM", MaterialTypePPE: "PPE",
	MaterialTypePS: "PS", MaterialTypePSU: "PSU", MaterialTypeTPI: "TPI",
	MaterialTypeSBS: "SBS", MaterialTypeOBC: "OBC", MaterialTypeEVA: "EVA",
	MaterialTypePA612: "PA612",
}

// materialSpec is a material as written to a tag: a spec material_type
// (nil = leave key 9 out) plus tags such as carbon fiber for composites.
type materialSpec struct {
	materialType *MaterialType
	tags         []uint16
}

func material(mt MaterialType, tags ...uint16) materialSpec {
	return materialSpec{materialType: &mt, tags: tags}
}

// legacyMaterialTypes maps the integer materialType values accepted by the
// write API before v0.8 (nfc-agent's own, non-spec numbering) to what they
// meant, so existing callers keep writing the material they intend - now
// encoded spec-correctly. Generic PA/Nylon uses PA12, matching SimplyPrint's
// OpenPrintTag encoder; composites use the base type plus additive tags.
var legacyMaterialTypes = map[int]materialSpec{
	0:   material(MaterialTypePLA),
	1:   material(MaterialTypeABS),
	2:   material(MaterialTypePETG),
	3:   material(MaterialTypeASA),
	4:   material(MaterialTypePC),
	5:   material(MaterialTypePA12), // Nylon
	6:   material(MaterialTypeTPU),
	7:   material(MaterialTypePVA),
	8:   material(MaterialTypeHIPS),
	9:   material(MaterialTypePP),
	10:  material(MaterialTypePEI),
	11:  material(MaterialTypePEEK),
	12:  material(MaterialTypePA12), // PA
	13:  material(MaterialTypePA12, TagContainsCarbonFiber, TagAbrasive),
	14:  material(MaterialTypePA12, TagContainsGlassFiber, TagAbrasive),
	15:  material(MaterialTypePLA, TagContainsCarbonFiber, TagAbrasive),
	16:  material(MaterialTypePLA, TagContainsGlassFiber, TagAbrasive),
	17:  material(MaterialTypePETG, TagContainsCarbonFiber, TagAbrasive),
	18:  material(MaterialTypePETG, TagContainsGlassFiber, TagAbrasive),
	255: {}, // Other: the spec says to leave material_type unspecified
}

// materialTypeAliases are names accepted on write in addition to the spec
// abbreviations (matched case-insensitively, ignoring "-", "_" and spaces).
var materialTypeAliases = map[string]materialSpec{
	"PA":     material(MaterialTypePA12),
	"NYLON":  material(MaterialTypePA12),
	"PACF":   material(MaterialTypePA12, TagContainsCarbonFiber, TagAbrasive),
	"PAGF":   material(MaterialTypePA12, TagContainsGlassFiber, TagAbrasive),
	"PLACF":  material(MaterialTypePLA, TagContainsCarbonFiber, TagAbrasive),
	"PLAGF":  material(MaterialTypePLA, TagContainsGlassFiber, TagAbrasive),
	"PETGCF": material(MaterialTypePETG, TagContainsCarbonFiber, TagAbrasive),
	"PETGGF": material(MaterialTypePETG, TagContainsGlassFiber, TagAbrasive),
	"OTHER":  {},
}

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
	GTIN                    uint64 `cbor:"4,keyasint,omitempty"`
	BrandSpecificInstanceID string `cbor:"5,keyasint,omitempty"`
	BrandSpecificPackageID  string `cbor:"6,keyasint,omitempty"`
	BrandSpecificMaterialID string `cbor:"7,keyasint,omitempty"`

	// Material classification (keys 8-11)
	MaterialClass MaterialClass `cbor:"8,keyasint,omitempty"`
	MaterialType  *MaterialType `cbor:"9,keyasint,omitempty"` // nil when not on the tag
	MaterialName  string        `cbor:"10,keyasint,omitempty"`
	BrandName     string        `cbor:"11,keyasint,omitempty"`

	// Protection and dates (keys 13-15)
	WriteProtection  uint16 `cbor:"13,keyasint,omitempty"`
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
	Tags []uint16 `cbor:"28,keyasint,omitempty"`

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
	MaterialAbbreviation string   `cbor:"52,keyasint,omitempty"`
	NominalFullLength    uint32   `cbor:"53,keyasint,omitempty"` // mm
	ActualFullLength     uint32   `cbor:"54,keyasint,omitempty"` // mm
	CountryOfOrigin      string   `cbor:"55,keyasint,omitempty"`
	Certifications       []uint16 `cbor:"56,keyasint,omitempty"`
}

// AuxSection contains mutable runtime data that printers can update.
// Keys 0-3 as per OpenPrintTag spec.
type AuxSection struct {
	ConsumedWeight     float32 `cbor:"0,keyasint,omitempty"`
	Workgroup          string  `cbor:"1,keyasint,omitempty"`
	GeneralPurposeUser string  `cbor:"2,keyasint,omitempty"`
	LastStirTime       uint32  `cbor:"3,keyasint,omitempty"` // Unix timestamp (for resin)
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
	MaterialName   string `json:"materialName,omitempty"`
	BrandName      string `json:"brandName,omitempty"`
	MaterialClass  string `json:"materialClass,omitempty"`
	MaterialType   string `json:"materialType,omitempty"`   // spec abbreviation, e.g. "PETG"
	MaterialTypeID *int   `json:"materialTypeId,omitempty"` // raw spec material_type value (key 9)
	Tags           []int  `json:"tags,omitempty"`           // spec tag values (key 28), e.g. 31 = contains carbon fiber

	// UUIDs as strings
	InstanceUUID string `json:"instanceUuid,omitempty"`
	PackageUUID  string `json:"packageUuid,omitempty"`
	MaterialUUID string `json:"materialUuid,omitempty"`
	BrandUUID    string `json:"brandUuid,omitempty"`

	// Identifiers (keys 4-7)
	GTIN                    uint64 `json:"gtin,omitempty"`
	BrandSpecificInstanceID string `json:"brandSpecificInstanceId,omitempty"`
	BrandSpecificPackageID  string `json:"brandSpecificPackageId,omitempty"`
	BrandSpecificMaterialID string `json:"brandSpecificMaterialId,omitempty"`
	MaterialAbbreviation    string `json:"materialAbbreviation,omitempty"` // key 52

	// Weight information
	NominalWeight   float32 `json:"nominalWeight,omitempty"`
	ConsumedWeight  float32 `json:"consumedWeight,omitempty"`
	RemainingWeight float32 `json:"remainingWeight,omitempty"`
	ActualWeight    float32 `json:"actualWeight,omitempty"` // actual netto full weight (key 17)
	SpoolWeight     float32 `json:"spoolWeight,omitempty"`  // empty container weight (key 18)

	// Physical properties
	PrimaryColor      string  `json:"primaryColor,omitempty"` // hex #RRGGBB or #RRGGBBAA
	FilamentDiameter  float32 `json:"filamentDiameter,omitempty"`
	FilamentLength    uint32  `json:"filamentLength,omitempty"`    // actual full length in mm (same as actualFullLength)
	ActualFullLength  uint32  `json:"actualFullLength,omitempty"`  // mm (key 54)
	NominalFullLength uint32  `json:"nominalFullLength,omitempty"` // mm (key 53)
	Density           float32 `json:"density,omitempty"`

	// Temperature settings
	MinPrintTemp   uint16 `json:"minPrintTemp,omitempty"`
	MaxPrintTemp   uint16 `json:"maxPrintTemp,omitempty"`
	PreheatTemp    uint16 `json:"preheatTemp,omitempty"`
	MinBedTemp     uint16 `json:"minBedTemp,omitempty"`
	MaxBedTemp     uint16 `json:"maxBedTemp,omitempty"`
	MinChamberTemp uint16 `json:"minChamberTemp,omitempty"`
	MaxChamberTemp uint16 `json:"maxChamberTemp,omitempty"`
	ChamberTemp    uint16 `json:"chamberTemp,omitempty"` // recommended chamber temperature (key 41)

	// Container dimensions (mm)
	ContainerWidth     uint16 `json:"containerWidth,omitempty"`
	ContainerOuterDiam uint16 `json:"containerOuterDiam,omitempty"`
	ContainerInnerDiam uint16 `json:"containerInnerDiam,omitempty"`
	ContainerHoleDiam  uint16 `json:"containerHoleDiam,omitempty"`

	// Dates
	ManufacturedDate uint32 `json:"manufacturedDate,omitempty"`
	ExpirationDate   uint32 `json:"expirationDate,omitempty"`

	// Auxiliary data
	Workgroup string `json:"workgroup,omitempty"`
}

// Input is the JSON structure for API write requests
type Input struct {
	// Required fields
	MaterialName  string  `json:"materialName"`
	BrandName     string  `json:"brandName"`
	MaterialClass int     `json:"materialClass"`
	NominalWeight float32 `json:"nominalWeight"`

	// MaterialType accepts a name (spec abbreviation such as "PETG", or an
	// alias such as "PA-CF") or, for backward compatibility, an integer in
	// nfc-agent's pre-v0.8 numbering (0=PLA, 1=ABS, 2=PETG, ...). Either way
	// the spec-correct value is written. Omitted = PLA, as before.
	MaterialType json.RawMessage `json:"materialType,omitempty"`
	// MaterialTypeID is a raw spec material_type value; overrides MaterialType.
	MaterialTypeID *int `json:"materialTypeId,omitempty"`
	// Tags are extra spec tag values (key 28), merged with any implied by MaterialType.
	Tags []int `json:"tags,omitempty"`

	// Optional fields
	InstanceUUID            string  `json:"instanceUuid,omitempty"`
	PackageUUID             string  `json:"packageUuid,omitempty"`
	MaterialUUID            string  `json:"materialUuid,omitempty"`
	BrandUUID               string  `json:"brandUuid,omitempty"`
	GTIN                    uint64  `json:"gtin,omitempty"`
	BrandSpecificInstanceID string  `json:"brandSpecificInstanceId,omitempty"`
	BrandSpecificPackageID  string  `json:"brandSpecificPackageId,omitempty"`
	BrandSpecificMaterialID string  `json:"brandSpecificMaterialId,omitempty"`
	MaterialAbbreviation    string  `json:"materialAbbreviation,omitempty"`
	FilamentDiameter        float32 `json:"filamentDiameter,omitempty"`
	PrimaryColor            string  `json:"primaryColor,omitempty"` // hex #RRGGBB or #RRGGBBAA
	Density                 float32 `json:"density,omitempty"`
	MinPrintTemp            uint16  `json:"minPrintTemp,omitempty"`
	MaxPrintTemp            uint16  `json:"maxPrintTemp,omitempty"`
	PreheatTemp             uint16  `json:"preheatTemp,omitempty"`
	MinBedTemp              uint16  `json:"minBedTemp,omitempty"`
	MaxBedTemp              uint16  `json:"maxBedTemp,omitempty"`
	MinChamberTemp          uint16  `json:"minChamberTemp,omitempty"`
	MaxChamberTemp          uint16  `json:"maxChamberTemp,omitempty"`
	ChamberTemp             uint16  `json:"chamberTemp,omitempty"`
	ConsumedWeight          float32 `json:"consumedWeight,omitempty"`
	ActualWeight            float32 `json:"actualWeight,omitempty"` // actual netto full weight
	SpoolWeight             float32 `json:"spoolWeight,omitempty"`  // empty container weight
	ContainerWidth          uint16  `json:"containerWidth,omitempty"`
	ContainerOuterDiam      uint16  `json:"containerOuterDiam,omitempty"`
	ContainerInnerDiam      uint16  `json:"containerInnerDiam,omitempty"`
	ContainerHoleDiam       uint16  `json:"containerHoleDiam,omitempty"`
	NominalFullLength       uint32  `json:"nominalFullLength,omitempty"` // mm
	ActualFullLength        uint32  `json:"actualFullLength,omitempty"`  // mm
	Workgroup               string  `json:"workgroup,omitempty"`
	ManufacturedDate        uint32  `json:"manufacturedDate,omitempty"`
	ExpirationDate          uint32  `json:"expirationDate,omitempty"`
}

// resolveMaterial turns the Input's materialType/materialTypeId/tags into
// what gets written to the tag.
func (i *Input) resolveMaterial() (materialSpec, error) {
	var m materialSpec
	switch {
	case i.MaterialTypeID != nil:
		if *i.MaterialTypeID < 0 || *i.MaterialTypeID > 0xFFFF {
			return m, fmt.Errorf("materialTypeId %d out of range", *i.MaterialTypeID)
		}
		m = material(MaterialType(*i.MaterialTypeID))
	case len(i.MaterialType) == 0 || string(i.MaterialType) == "null":
		if i.MaterialClass == int(MaterialClassFFF) {
			m = material(MaterialTypePLA) // pre-v0.8 default (missing int = 0 = PLA)
		} // material_type is only defined for FFF; leave it out for SLA
	default:
		var name string
		var legacy int
		if err := json.Unmarshal(i.MaterialType, &name); err == nil {
			found, ok := materialTypeByName(name)
			if !ok {
				return m, fmt.Errorf("unknown materialType %q", name)
			}
			m = found
		} else if err := json.Unmarshal(i.MaterialType, &legacy); err == nil {
			if found, ok := legacyMaterialTypes[legacy]; ok {
				m = found
			} else if legacy >= 0 && legacy <= 0xFF {
				m = material(MaterialType(legacy)) // undefined in the old numbering: written as-is, like before
			} else {
				return m, fmt.Errorf("materialType %d out of range", legacy)
			}
		} else {
			return m, fmt.Errorf("materialType must be a name or an integer")
		}
	}

	m.tags = append([]uint16{}, m.tags...)
	for _, t := range i.Tags {
		if t < 0 || t > 0xFFFF {
			return m, fmt.Errorf("tag %d out of range", t)
		}
		if !containsTag(m.tags, uint16(t)) {
			m.tags = append(m.tags, uint16(t))
		}
	}
	return m, nil
}

// Validate reports input errors that would make Encode fail, so API
// handlers can reject bad requests before touching the card.
func (i *Input) Validate() error {
	_, err := i.Encode()
	return err
}

// materialTypeByName looks up a spec abbreviation or alias, ignoring case,
// "-", "_" and spaces.
func materialTypeByName(name string) (materialSpec, bool) {
	key := strings.ToUpper(strings.NewReplacer("-", "", "_", "", " ", "").Replace(name))
	for mt, n := range materialTypeNames {
		if n == key {
			return material(mt), true
		}
	}
	m, ok := materialTypeAliases[key]
	return m, ok
}

func containsTag(tags []uint16, t uint16) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}

// ToResponse converts internal OpenPrintTag to API response
func (o *OpenPrintTag) ToResponse() *Response {
	resp := &Response{
		MaterialName:            o.Main.MaterialName,
		BrandName:               o.Main.BrandName,
		MaterialClass:           materialClassToString(o.Main.MaterialClass),
		GTIN:                    o.Main.GTIN,
		BrandSpecificInstanceID: o.Main.BrandSpecificInstanceID,
		BrandSpecificPackageID:  o.Main.BrandSpecificPackageID,
		BrandSpecificMaterialID: o.Main.BrandSpecificMaterialID,
		MaterialAbbreviation:    o.Main.MaterialAbbreviation,
		NominalWeight:           o.Main.NominalNettoFullWeight,
		ActualWeight:            o.Main.ActualNettoFullWeight,
		SpoolWeight:             o.Main.EmptyContainerWeight,
		ConsumedWeight:          o.Aux.ConsumedWeight,
		FilamentDiameter:        o.Main.FilamentDiameter,
		FilamentLength:          o.Main.ActualFullLength,
		ActualFullLength:        o.Main.ActualFullLength,
		NominalFullLength:       o.Main.NominalFullLength,
		Density:                 o.Main.Density,
		MinPrintTemp:            o.Main.MinPrintTemp,
		MaxPrintTemp:            o.Main.MaxPrintTemp,
		PreheatTemp:             o.Main.PreheatTemp,
		MinBedTemp:              o.Main.MinBedTemp,
		MaxBedTemp:              o.Main.MaxBedTemp,
		MinChamberTemp:          o.Main.MinChamberTemp,
		MaxChamberTemp:          o.Main.MaxChamberTemp,
		ChamberTemp:             o.Main.ChamberTemp,
		ContainerWidth:          o.Main.ContainerWidth,
		ContainerOuterDiam:      o.Main.ContainerOuterDiameter,
		ContainerInnerDiam:      o.Main.ContainerInnerDiameter,
		ContainerHoleDiam:       o.Main.ContainerHoleDiameter,
		ManufacturedDate:        o.Main.ManufacturedDate,
		ExpirationDate:          o.Main.ExpirationDate,
		Workgroup:               o.Aux.Workgroup,
	}

	if o.Main.MaterialType != nil {
		resp.MaterialType = materialTypeToString(*o.Main.MaterialType)
		id := int(*o.Main.MaterialType)
		resp.MaterialTypeID = &id
	}
	for _, t := range o.Main.Tags {
		resp.Tags = append(resp.Tags, int(t))
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

// ToResponseForTag converts to an API response like ToResponse, and fills in
// UUIDs that aren't stored on the tag by deriving them as the spec requires
// readers to (nfc_data_format 3.2): brand from brand_name, material from
// brand_uuid + material_name, package from brand_uuid + gtin, and instance
// from the NFC tag UID. Factory tags such as Prusament omit these fields.
// tagUID must be MSB-first (nil if unknown).
func (o *OpenPrintTag) ToResponseForTag(tagUID []byte) *Response {
	resp := o.ToResponse()

	brandUUID := o.Main.BrandUUID
	if len(brandUUID) != 16 && o.Main.BrandName != "" {
		brandUUID = GenerateBrandUUID(o.Main.BrandName)
		resp.BrandUUID = formatUUID(brandUUID)
	}
	if len(brandUUID) == 16 {
		if resp.MaterialUUID == "" && o.Main.MaterialName != "" {
			resp.MaterialUUID = formatUUID(generateMaterialUUIDFromBrand(brandUUID, o.Main.MaterialName))
		}
		if resp.PackageUUID == "" && o.Main.GTIN != 0 {
			resp.PackageUUID = formatUUID(GeneratePackageUUID(brandUUID, strconv.FormatUint(o.Main.GTIN, 10)))
		}
	}
	if resp.InstanceUUID == "" && len(tagUID) > 0 {
		resp.InstanceUUID = formatUUID(GenerateInstanceUUID(tagUID))
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

// legacyOtherMaterialType is what nfc-agent wrote for "Other" before v0.8.
// It isn't a spec value, so it can only come from those tags.
const legacyOtherMaterialType MaterialType = 255

// materialTypeToString converts a spec MaterialType value to its abbreviation
func materialTypeToString(mt MaterialType) string {
	if name, ok := materialTypeNames[mt]; ok {
		return name
	}
	if mt == legacyOtherMaterialType {
		return "Other"
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
