package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/cmd"
)

// Build-time variables set by goreleaser/ldflags
// CalVer format: YYYY.MM.BUILD (e.g., 2026.01.1)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	defaultConfigDirName = ".tunatap"
)

func getHomeFolder() (string, error) {
	current, e := user.Current()
	if e != nil {
		// Give up and try to return something sensible
		home := os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}

		if home == "" {
			return "", fmt.Errorf("could not determine home directory")
		}

		return home, nil
	}
	return current.HomeDir, nil
}

func createTunatapDirectory() string {
	// Get the user's home directory
	homeDir, err := getHomeFolder()
	if err != nil {
		log.Fatal().Msgf("Failed to get user home directory: %v", err)
	}

	// Construct the full directory path
	dirPath := filepath.Join(homeDir, defaultConfigDirName)

	// Check if the directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// Directory does not exist, create it
		err = os.Mkdir(dirPath, 0755)
		if err != nil {
			log.Fatal().Msgf("Failed to create directory %s: %v", dirPath, err)
		}
	} else if err != nil {
		log.Fatal().Msgf("Failed to check directory %s: %v", dirPath, err)
	}

	return dirPath
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	homePath := createTunatapDirectory()

	cmd.SetVersionInfo(version, commit, date)
	cmd.SetHomePath(homePath)
	cmd.Execute()
}
