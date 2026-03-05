package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const toolName = "document-tools"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", toolName, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)

	inputPath := fs.String("i", "", "input file path (optional: reads stdin markdown)")
	outputPath := fs.String("o", "", "output file path")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s -i input.ext -o output.ext\n", toolName)
		fmt.Fprintf(stderr, "   or: %s -o output.ext   # reads markdown from stdin\n", toolName)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 0 {
		fs.Usage()
		return errors.New("unexpected positional arguments")
	}

	out := strings.TrimSpace(*outputPath)
	if out == "" {
		return errors.New("-o output path is required")
	}

	target, err := targetFromOutputPath(out)
	if err != nil {
		return err
	}

	input, source, err := loadInput(*inputPath, stdin)
	if err != nil {
		return err
	}

	if err := writeOutput(source, input, target, out); err != nil {
		return err
	}
	return nil
}

func loadInput(inputPath string, stdin io.Reader) ([]byte, string, error) {
	if inputPath == "" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, "", fmt.Errorf("read stdin: %w", err)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			return nil, "", errors.New("stdin is empty")
		}
		return b, "markdown", nil
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(inputPath), "."))
	switch ext {
	case "pdf":
		if _, err := exec.LookPath("pdftotext"); err != nil {
			return nil, "", errors.New("pdftotext is required for PDF input")
		}

		cmd := exec.Command("pdftotext", "-layout", inputPath, "-")
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut

		if err := cmd.Run(); err != nil {
			return nil, "", fmt.Errorf("pdftotext failed: %v: %s", err, strings.TrimSpace(errOut.String()))
		}

		return out.Bytes(), "markdown", nil
	default:
		b, err := os.ReadFile(inputPath)
		if err != nil {
			return nil, "", fmt.Errorf("read input file: %w", err)
		}
		return b, formatFromExt(ext), nil
	}
}

func writeOutput(source string, input []byte, target, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil && filepath.Dir(outputPath) != "." {
		return fmt.Errorf("prepare output directory: %w", err)
	}

	if target == "markdown" && source == "markdown" {
		if err := os.WriteFile(outputPath, input, 0o644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}

	if _, err := exec.LookPath("pandoc"); err != nil {
		return errors.New("pandoc is required for this conversion")
	}

	cmd := exec.Command("pandoc", "-f", source, "-t", target, "-o", outputPath)
	cmd.Stdin = bytes.NewReader(input)

	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc failed: %v: %s", err, strings.TrimSpace(errOut.String()))
	}

	return nil
}

func targetFromOutputPath(outputPath string) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(outputPath), "."))
	if ext == "" {
		return "", errors.New("output file must include an extension")
	}
	return formatFromExt(ext), nil
}

func formatFromExt(ext string) string {
	switch ext {
	case "", "md":
		return "markdown"
	case "htm":
		return "html"
	default:
		return ext
	}
}
