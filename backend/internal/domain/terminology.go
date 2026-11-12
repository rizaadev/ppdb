package domain

// TerminologyPreset identifies which UI vocabulary template a tenant uses.
type TerminologyPreset string

const (
	PresetSPMB    TerminologyPreset = "spmb"
	PresetPMBM    TerminologyPreset = "pmbm"
	PresetMandiri TerminologyPreset = "mandiri"
)

// InstitutionType classifies the educational entity owning a tenant workspace.
type InstitutionType string

const (
	InstitutionSchool     InstitutionType = "school"
	InstitutionMadrasah   InstitutionType = "madrasah"
	InstitutionFoundation InstitutionType = "foundation"
	InstitutionPesantren  InstitutionType = "pesantren"
)

