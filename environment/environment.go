package environment

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
)

func LoadEnvironmentFile(others ...string) error {
	envFile := ".env"

	err := loadFile(envFile)
	if err != nil {
		return err
	}

	overrideEnvFile := ".override.env"

	err = loadFile(overrideEnvFile)
	if err != nil {
		return err
	}

	for _, filePath := range others {
		if filePath == "" {
			continue
		}

		err = loadFile(filePath)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadFile(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("error checking %s file: %w", filePath, err)
	}

	if info.IsDir() {
		return nil
	}

	variables, err := godotenv.Read(filePath)
	if err != nil {
		return fmt.Errorf("error loading %s file: %w", filePath, err)
	}

	// Reject malformed entries before applying any variables from this file.
	for key, value := range variables {
		if key == "" || (runtime.GOOS != "plan9" && strings.ContainsRune(value, 0)) {
			return fmt.Errorf(
				"invalid environment variable %q in %s: %w",
				key,
				filePath,
				os.ErrInvalid,
			)
		}
	}

	for key, value := range variables {
		err = os.Setenv(key, value)
		if err != nil {
			return fmt.Errorf(
				"error setting environment variable %q from %s: %w",
				key,
				filePath,
				err,
			)
		}
	}

	return nil
}
