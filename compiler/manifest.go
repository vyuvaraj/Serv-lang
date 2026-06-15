package compiler

import (
	"bufio"
	"os"
	"strings"
)

type ProjectManifest struct {
	Name    string
	Version string
	Entry   string
}

func ParseManifest(path string) (*ProjectManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	manifest := &ProjectManifest{
		Entry: "main.srv", // default entry point
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Ignore section headers for now
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			val = val[1 : len(val)-1]
		}

		switch key {
		case "name":
			manifest.Name = val
		case "version":
			manifest.Version = val
		case "entry":
			manifest.Entry = val
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return manifest, nil
}
