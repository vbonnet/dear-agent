//go:build linux

package main

import (
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func validateProcessImage(pid int) error {
	file, err := os.Open(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	image, err := elf.NewFile(file)
	if err != nil {
		return fmt.Errorf("parse launcher ELF image: %w", err)
	}
	defer func() { _ = image.Close() }()
	return validateELFProgramHeaders(image.Progs)
}

func validateELFProgramHeaders(programs []*elf.Prog) error {
	for _, program := range programs {
		if program.Type == elf.PT_INTERP {
			return errors.New(
				"launcher uses a dynamic ELF interpreter; reinstall the cgo-free authenticated build",
			)
		}
	}
	return nil
}

func processParentPID(pid int) (int, error) {
	file, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		return strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("process status has no parent PID")
}

func processExecutableSHA256(pid int) (string, error) {
	file, err := os.Open(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("launcher executable image is not a regular file")
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sum.Sum(nil)), nil
}

func processExecutablePath(pid int) (string, error) {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(path, " (deleted)") {
		return "", errors.New("process executable was replaced after launch")
	}
	return path, nil
}

func processCodeIdentity(pid int) (string, error) {
	digest, err := processExecutableSHA256(pid)
	if err != nil {
		return "", err
	}
	return codeIdentityAlgorithm() + ":" + digest, nil
}

func codeIdentityAlgorithm() string { return "linux-sha256" }
func codeIdentityHexLength() int    { return sha256.Size * 2 }
