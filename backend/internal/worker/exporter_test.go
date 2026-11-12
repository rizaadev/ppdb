package worker

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestBuildNationalExportCSVAndXLSX(t *testing.T) {
	rows := []ExportStudentRow{
		{
			RegistrationNo:  "DOMI-26-0001",
			Status:          "enrolled",
			TrackCode:       "domisili",
			TrackName:       "Jalur Domisili",
			FullName:        "Zaidan Al-Farizi & Friends",
			NIK:             "3175011408110011",
			NISN:            "0098765432",
			BirthPlace:      "Jakarta",
			BirthDate:       "2010-08-14",
			Gender:          "L",
			FatherName:      "H. Ahmad Fauzan",
			ContactWhatsApp: "+6281234567890",
			RankPosition:    "1",
		},
	}

	csvName, csvMime, csvBytes, err := BuildNationalExport("custom_csv", "sma-1", rows)
	if err != nil {
		t.Fatalf("custom_csv failed: %v", err)
	}
	if csvName != "ekspor-pendaftar-sma-1.csv" || !strings.Contains(csvMime, "text/csv") {
		t.Fatalf("unexpected csv meta: %s %s", csvName, csvMime)
	}
	if !strings.Contains(string(csvBytes), "Zaidan Al-Farizi & Friends") || !strings.Contains(string(csvBytes), "DOMI-26-0001") {
		t.Fatalf("csv content missing student row: %s", string(csvBytes))
	}

	xlsxName, xlsxMime, xlsxBytes, err := BuildNationalExport("dapodik_xlsx", "sma-1", rows)
	if err != nil {
		t.Fatalf("dapodik_xlsx failed: %v", err)
	}
	if xlsxName != "ekspor-dapodik-sma-1.xlsx" || !strings.Contains(xlsxMime, "spreadsheetml") {
		t.Fatalf("unexpected xlsx meta: %s %s", xlsxName, xlsxMime)
	}

	zr, err := zip.NewReader(bytes.NewReader(xlsxBytes), int64(len(xlsxBytes)))
	if err != nil {
		t.Fatalf("invalid xlsx zip archive: %v", err)
	}

	foundSheet := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		dec := xml.NewDecoder(rc)
		for {
			if _, err := dec.Token(); err != nil {
				if err.Error() == "EOF" {
					break
				}
				_ = rc.Close()
				t.Fatalf("invalid XML in %s: %v", f.Name, err)
			}
		}
		_ = rc.Close()
		if f.Name == "xl/worksheets/sheet1.xml" {
			foundSheet = true
		}
	}
	if !foundSheet {
		t.Fatal("missing xl/worksheets/sheet1.xml inside xlsx archive")
	}
}

