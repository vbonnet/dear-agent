package main

import "testing"

// Operation tests

func TestAdd(t *testing.T) {
	result := add(5, 3)
	if result != 8 {
		t.Errorf("add(5, 3) = %v; want 8", result)
	}
}

func TestSubtract(t *testing.T) {
	result := subtract(10, 3)
	if result != 7 {
		t.Errorf("subtract(10, 3) = %v; want 7", result)
	}
}

func TestMultiply(t *testing.T) {
	result := multiply(4, 5)
	if result != 20 {
		t.Errorf("multiply(4, 5) = %v; want 20", result)
	}
}

func TestDivide(t *testing.T) {
	result, err := divide(10, 2)
	if err != nil {
		t.Errorf("divide(10, 2) unexpected error: %v", err)
	}
	if result != 5 {
		t.Errorf("divide(10, 2) = %v; want 5", result)
	}
}

func TestDivideByZero(t *testing.T) {
	_, err := divide(10, 0)
	if err == nil {
		t.Error("divide(10, 0) should return error")
	}
}

// Validation tests

func TestValidateNoOperation(t *testing.T) {
	err := validate(false, false, false, false, []string{"5", "3"})
	if err == nil {
		t.Error("validate should error when no operation specified")
	}
}

func TestValidateMultipleOperations(t *testing.T) {
	err := validate(true, true, false, false, []string{"5", "3"})
	if err == nil {
		t.Error("validate should error when multiple operations specified")
	}
}

func TestValidateWrongArgCount(t *testing.T) {
	err := validate(true, false, false, false, []string{"5"})
	if err == nil {
		t.Error("validate should error when argument count != 2")
	}
}

func TestValidateInvalidNumber(t *testing.T) {
	err := validate(true, false, false, false, []string{"foo", "3"})
	if err == nil {
		t.Error("validate should error when arguments are not numeric")
	}
}

func TestValidateSuccess(t *testing.T) {
	err := validate(true, false, false, false, []string{"5", "3"})
	if err != nil {
		t.Errorf("validate should succeed with valid inputs, got error: %v", err)
	}
}
