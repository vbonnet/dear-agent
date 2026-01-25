package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

// Operation functions

func add(a, b float64) float64 {
	return a + b
}

func subtract(a, b float64) float64 {
	return a - b
}

func multiply(a, b float64) float64 {
	return a * b
}

func divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, fmt.Errorf("Cannot divide by zero")
	}
	return a / b, nil
}

// Validation function

func validate(add, subtract, multiply, divide bool, args []string) error {
	// Count operations
	opCount := 0
	if add {
		opCount++
	}
	if subtract {
		opCount++
	}
	if multiply {
		opCount++
	}
	if divide {
		opCount++
	}

	// Validate exactly one operation
	if opCount == 0 {
		return fmt.Errorf("Must specify exactly one operation (--add, --subtract, --multiply, --divide)")
	}
	if opCount > 1 {
		return fmt.Errorf("Cannot specify multiple operations")
	}

	// Validate argument count
	if len(args) != 2 {
		return fmt.Errorf("Expected 2 numeric arguments, got %d", len(args))
	}

	// Validate numeric arguments
	for _, arg := range args {
		if _, err := strconv.ParseFloat(arg, 64); err != nil {
			return fmt.Errorf("Arguments must be valid numbers")
		}
	}

	return nil
}

// Main function

func main() {
	// Define flags
	addFlag := flag.Bool("add", false, "Add two numbers")
	subtractFlag := flag.Bool("subtract", false, "Subtract two numbers")
	multiplyFlag := flag.Bool("multiply", false, "Multiply two numbers")
	divideFlag := flag.Bool("divide", false, "Divide two numbers")
	flag.Parse()

	// Get remaining args
	args := flag.Args()

	// Validate
	if err := validate(*addFlag, *subtractFlag, *multiplyFlag, *divideFlag, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Parse numbers
	a, _ := strconv.ParseFloat(args[0], 64)
	b, _ := strconv.ParseFloat(args[1], 64)

	// Dispatch to operation
	var result float64
	var err error

	switch {
	case *addFlag:
		result = add(a, b)
	case *subtractFlag:
		result = subtract(a, b)
	case *multiplyFlag:
		result = multiply(a, b)
	case *divideFlag:
		result, err = divide(a, b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println(result)
}
