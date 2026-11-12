package worker

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// ExportStudentRow represents one enrolled/accepted student row formatted for Dapodik, EMIS, or CSV export.
type ExportStudentRow struct {
	RegistrationNo    string
	Status            string
	TrackCode         string
	TrackName         string
	FullName          string
	NIK               string
	KKNumber          string
	NISN              string
	BirthPlace        string
	BirthDate         string
	Gender            string
	Religion          string
	Citizenship       string
	ChildOrder        string
	SiblingsCount     string
	SpecialNeedsCode  string
	AddressStreet     string
	RT                string
	RW                string
	Hamlet            string
	VillageName       string
	DistrictName      string
	RegencyName       string
	ProvinceName      string
	PostalCode        string
	Latitude          string
	Longitude         string
	PrevSchoolName    string
	PrevSchoolNPSN    string
	GraduationYear    string
	IjazahNumber      string
	FatherName        string
	FatherNIK         string
	FatherOccupation  string
	FatherIncome      string
	MotherName        string
	MotherNIK         string
	MotherOccupation  string
	MotherIncome      string
	GuardianName      string
	GuardianNIK       string
	ContactWhatsApp   string
	FinalScore        string
	RankPosition      string
}

var exportHeaders = []string{
	"No Pendaftaran",
	"Status",
	"Kode Jalur",
	"Nama Jalur",
	"Nama Lengkap",
	"NIK",
	"Nomor KK",
	"NISN",
	"Tempat Lahir",
	"Tanggal Lahir",
	"Jenis Kelamin",
	"Agama",
	"Kewarganegaraan",
	"Anak Ke",
	"Jumlah Saudara",
	"Kode Kebutuhan Khusus",
	"Alamat Jalan",
	"RT",
	"RW",
	"Dusun",
	"Desa / Kelurahan",
	"Kecamatan",
	"Kabupaten / Kota",
	"Provinsi",
	"Kode Pos",
	"Lintang",
	"Bujur",
	"Asal Sekolah",
	"NPSN Asal",
	"Tahun Lulus",
	"Nomor Ijazah",
	"Nama Ayah",
	"NIK Ayah",
	"Pekerjaan Ayah",
	"Penghasilan Ayah",
	"Nama Ibu",
	"NIK Ibu",
	"Pekerjaan Ibu",
	"Penghasilan Ibu",
	"Nama Wali",
	"NIK Wali",
	"No WhatsApp Utama",
	"Nilai Akhir",
	"Peringkat",
}

func rowToSlice(r ExportStudentRow) []string {
	return []string{
		r.RegistrationNo, r.Status, r.TrackCode, r.TrackName,
		r.FullName, r.NIK, r.KKNumber, r.NISN, r.BirthPlace, r.BirthDate,
		r.Gender, r.Religion, r.Citizenship, r.ChildOrder, r.SiblingsCount, r.SpecialNeedsCode,
		r.AddressStreet, r.RT, r.RW, r.Hamlet, r.VillageName, r.DistrictName,
		r.RegencyName, r.ProvinceName, r.PostalCode, r.Latitude, r.Longitude,
		r.PrevSchoolName, r.PrevSchoolNPSN, r.GraduationYear, r.IjazahNumber,
		r.FatherName, r.FatherNIK, r.FatherOccupation, r.FatherIncome,
		r.MotherName, r.MotherNIK, r.MotherOccupation, r.MotherIncome,
		r.GuardianName, r.GuardianNIK, r.ContactWhatsApp, r.FinalScore, r.RankPosition,
	}
}

// BuildNationalExport renders Dapodik XLSX, EMIS XLSX, or UTF-8 CSV bytes ready for private S3 storage or download.
func BuildNationalExport(format, unitSlug string, rows []ExportStudentRow) (filename, mimeType string, content []byte, err error) {
	safeUnit := strings.ToLower(strings.TrimSpace(unitSlug))
	if safeUnit == "" {
		safeUnit = "unit"
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "custom_csv":
		var buf bytes.Buffer
		// UTF-8 BOM ensures Microsoft Excel on Windows opens Indonesian characters cleanly.
		buf.Write([]byte{0xEF, 0xBB, 0xBF})
		w := csv.NewWriter(&buf)
		if err := w.Write(exportHeaders); err != nil {
			return "", "", nil, err
		}
		for _, r := range rows {
			if err := w.Write(rowToSlice(r)); err != nil {
				return "", "", nil, err
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return "", "", nil, err
		}
		return fmt.Sprintf("ekspor-pendaftar-%s.csv", safeUnit), "text/csv; charset=utf-8", buf.Bytes(), nil

	case "dapodik_xlsx", "emis_xlsx":
		sheetTitle := "Dapodik_Peserta_Didik"
		prefix := "dapodik"
		if strings.ToLower(format) == "emis_xlsx" {
			sheetTitle = "EMIS_Calon_Santri_Murid"
			prefix = "emis"
		}
		allRows := make([][]string, 0, len(rows)+1)
		allRows = append(allRows, exportHeaders)
		for _, r := range rows {
			allRows = append(allRows, rowToSlice(r))
		}
		rawXLSX, err := buildSimpleXLSX(sheetTitle, allRows)
		if err != nil {
			return "", "", nil, err
		}
		return fmt.Sprintf("ekspor-%s-%s.xlsx", prefix, safeUnit),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			rawXLSX,
			nil

	default:
		return "", "", nil, fmt.Errorf("unsupported export format %q", format)
	}
}

func buildSimpleXLSX(sheetName string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	addFile := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}

	if err := addFile("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`); err != nil {
		return nil, err
	}

	if err := addFile("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`); err != nil {
		return nil, err
	}

	if err := addFile("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`); err != nil {
		return nil, err
	}

	if err := addFile("xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="2">
    <font><sz val="11"/><name val="Calibri"/></font>
    <font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/></font>
  </fonts>
  <fills count="3">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
    <fill><patternFill patternType="solid"><fgColor rgb="FF1E3A5F"/></patternFill></fill>
  </fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="2">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/>
  </cellXfs>
</styleSheet>`); err != nil {
		return nil, err
	}

	cleanSheetName := sanitizeXMLText(sheetName)
	if len(cleanSheetName) > 31 {
		cleanSheetName = cleanSheetName[:31]
	}
	var escSheet bytes.Buffer
	_ = xml.EscapeText(&escSheet, []byte(cleanSheetName))

	if err := addFile("xl/workbook.xml", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="%s" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`, escSheet.String())); err != nil {
		return nil, err
	}

	var sheetXML strings.Builder
	sheetXML.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheetXML.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)

	for rIdx, cols := range rows {
		rowNum := rIdx + 1
		sheetXML.WriteString(`<row r="`)
		sheetXML.WriteString(strconv.Itoa(rowNum))
		sheetXML.WriteString(`">`)
		for cIdx, val := range cols {
			ref := colLetters(cIdx) + strconv.Itoa(rowNum)
			styleAttr := ""
			if rIdx == 0 {
				styleAttr = ` s="1"`
			}
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(sanitizeXMLText(val)))
			sheetXML.WriteString(`<c r="`)
			sheetXML.WriteString(ref)
			sheetXML.WriteString(`"`)
			sheetXML.WriteString(styleAttr)
			sheetXML.WriteString(` t="inlineStr"><is><t xml:space="preserve">`)
			sheetXML.WriteString(escaped.String())
			sheetXML.WriteString(`</t></is></c>`)
		}
		sheetXML.WriteString(`</row>`)
	}
	sheetXML.WriteString(`</sheetData></worksheet>`)

	if err := addFile("xl/worksheets/sheet1.xml", sheetXML.String()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func colLetters(idx int) string {
	result := ""
	n := idx
	for {
		rem := n % 26
		result = string(rune('A'+rem)) + result
		n = n/26 - 1
		if n < 0 {
			break
		}
	}
	return result
}

func sanitizeXMLText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) {
			return r
		}
		return -1
	}, s)
}

