package imagemeta

var iptcFieldMap = map[uint8]iptcField{
	0:   {"RecordVersion", false, "B"},
	5:   {"ObjectName", false, "string"},
	7:   {"EditStatus", false, "string"},
	10:  {"Urgency", false, "B"},
	15:  {"Category", true, "string"},
	20:  {"SupplementalCategory", true, "string"},
	22:  {"FixtureIdentifier", false, "string"},
	25:  {"Keywords", true, "string"},
	26:  {"ContentLocationCode", false, "string"},
	27:  {"ContentLocationName", false, "string"},
	30:  {"ReleaseDate", false, "string"},
	35:  {"ReleaseTime", false, "string"},
	37:  {"ExpirationDate", false, "string"},
	38:  {"ExpirationTime", false, "string"},
	40:  {"SpecialInstructions", false, "string"},
	42:  {"ActionAdvised", false, "B"},
	45:  {"ReferenceService", false, "string"},
	47:  {"ReferenceDate", false, "string"},
	50:  {"ReferenceNumber", false, "string"},
	55:  {"DateCreated", false, "string"},
	60:  {"TimeCreated", false, "string"},
	62:  {"DigitalCreationDate", false, "string"},
	63:  {"DigitalCreationTime", false, "string"},
	65:  {"OriginatingProgram", false, "string"},
	70:  {"ProgramVersion", false, "string"},
	75:  {"ObjectCycle", false, "string"},
	80:  {"Byline", false, "string"},
	85:  {"BylineTitle", false, "string"},
	90:  {"City", false, "string"},
	92:  {"SubLocation", false, "string"},
	95:  {"ProvinceState", false, "string"},
	100: {"CountryCode", false, "string"},
	101: {"CountryName", false, "string"},
	103: {"OriginalTransmissionReference", false, "string"},
	105: {"Headline", false, "string"},
	110: {"Credit", false, "string"},
	115: {"Source", false, "string"},
	116: {"Copyright", false, "string"},
	118: {"Contact", false, "string"},
}

type iptcField struct {
	name       string
	repeatable bool
	format     string
}
