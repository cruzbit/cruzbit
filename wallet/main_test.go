package main

import (
	. "cruzbit"
	"strconv"
	"testing"
)

func TestRoundFloat(t *testing.T) {
	value, _ := strconv.ParseFloat("4.89", 64)
	amount := int64(roundFloat(value, 8) * CRUZBITS_PER_CRUZ)
	if amount != 489000000 {
		t.Fatalf("Expected %d, got %d\n", 489000000, amount)
	}
	f := roundFloat(float64(amount), 8) / CRUZBITS_PER_CRUZ
	if f != 4.89 {
		t.Fatalf("Expected 4.89, got: %v\n", f)
	}

	value, _ = strconv.ParseFloat("0.00000001", 64)
	amount = int64(roundFloat(value, 8) * CRUZBITS_PER_CRUZ)
	if amount != 1 {
		t.Fatalf("Expected %d, got %d\n", 1, amount)
	}
	f = roundFloat(float64(amount), 8) / CRUZBITS_PER_CRUZ
	if f != 0.00000001 {
		t.Fatalf("Expected 0.00000001, got: %v\n", f)
	}

	value, _ = strconv.ParseFloat("1.00000001", 64)
	amount = int64(roundFloat(value, 8) * CRUZBITS_PER_CRUZ)
	if amount != 100000001 {
		t.Fatalf("Expected %d, got %d\n", 100000001, amount)
	}
	f = roundFloat(float64(amount), 8) / CRUZBITS_PER_CRUZ
	if f != 1.00000001 {
		t.Fatalf("Expected 1.00000001, got: %v\n", f)
	}

	value, _ = strconv.ParseFloat("123", 64)
	amount = int64(roundFloat(value, 8) * CRUZBITS_PER_CRUZ)
	if amount != 12300000000 {
		t.Fatalf("Expected %d, got %d\n", 12300000000, amount)
	}
	f = roundFloat(float64(amount), 8) / CRUZBITS_PER_CRUZ
	if f != 123.0 {
		t.Fatalf("Expected 123.0, got: %v\n", f)
	}
}
