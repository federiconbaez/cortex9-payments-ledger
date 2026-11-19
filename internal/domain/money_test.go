package domain

import "testing"

func TestParseCurrency(t *testing.T) {
	c, err := ParseCurrency("ars")
	if err != nil || c != "ARS" {
		t.Fatalf("%q %v", c, err)
	}
	if _, err := ParseCurrency("AR"); err == nil {
		t.Fatal("se esperaba error")
	}
	if _, err := ParseCurrency("AR$"); err == nil {
		t.Fatal("se esperaba error")
	}
}

func TestMoneyValidate(t *testing.T) {
	if err := (Money{Currency: "ARS", Amount: 1}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Money{Currency: "ARS", Amount: 0}).Validate(); err == nil {
		t.Fatal("cero")
	}
	if err := (Money{Currency: "AR", Amount: 1}).Validate(); err == nil {
		t.Fatal("moneda")
	}
}
