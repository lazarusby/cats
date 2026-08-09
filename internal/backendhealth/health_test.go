package backendhealth

import "testing"

func TestReportValidation(t *testing.T) {
	if err := (Report{Product: Product, Version: "abc1234"}).Validate("abc1234"); err != nil {
		t.Fatal(err)
	}
	for _, report := range []Report{
		{Product: "other", Version: "abc1234"},
		{Product: Product},
		{Product: Product, Version: "old"},
	} {
		if err := report.Validate("abc1234"); err == nil {
			t.Fatalf("Validate(%#v) unexpectedly succeeded", report)
		}
	}
}
