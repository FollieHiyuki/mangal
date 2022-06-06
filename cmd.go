package main

import (
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
)

var rootCmd = &cobra.Command{
	Use:   strings.ToLower(AppName),
	Short: AppName + " - Manga Downloader",
	Long:  `A fast and flexible manga downloader`,
	Run: func(cmd *cobra.Command, args []string) {
		config, _ := cmd.Flags().GetString("config")
		exists, err := Afero.Exists(config)

		if err != nil {
			log.Fatal(errors.New("access to config file denied"))
		}

		if config != "" {
			config = path.Clean(config)
			if !exists {
				log.Fatal(errors.New(fmt.Sprintf("config at path %s doesn't exist", config)))
			}

			UserConfig = GetConfig(config)
		} else {
			UserConfig = GetConfig("") // get config from default config path
		}

		var program *tea.Program

		if UserConfig.Fullscreen {
			program = tea.NewProgram(newBubble(searchState), tea.WithAltScreen())
		} else {
			program = tea.NewProgram(newBubble(searchState))
		}

		if err := program.Start(); err != nil {
			log.Fatal(err)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Long:  fmt.Sprintf("Shows %s versions and build date", AppName),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", AppName, version)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update " + AppName,
	Long:  "Fetches new version from github and reinstalls it " + AppName,
	Run: func(cmd *cobra.Command, args []string) {
		// Get mod name
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			log.Fatal(failStyle.Render("Failed to read build info"))
		}

		modName := bi.Path
		command := exec.Command("go", "install", modName+"@latest")

		err := command.Start()

		if err != nil {
			log.Fatal(failStyle.Render("Update failed"))
		} else {
			fmt.Println(successStyle.Render("Updated"))
		}
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove cached and temp files",
	Long:  "Removes cached files produced by scraper and temp files from downloader",
	Run: func(cmd *cobra.Command, args []string) {
		var (
			// counter of removed files
			counter int
			// bytes removed
			bytes int64
		)

		// Cleanup temp files
		tempDir := os.TempDir()
		tempFiles, err := Afero.ReadDir(tempDir)
		if err == nil {
			lowerAppName := strings.ToLower(AppName)
			for _, tempFile := range tempFiles {
				name := tempFile.Name()
				if strings.HasPrefix(name, AppName) || strings.HasPrefix(name, lowerAppName) {
					p := filepath.Join(tempDir, name)

					if tempFile.IsDir() {
						b, err := DirSize(p)
						if err == nil {
							bytes += b
						}
					}

					err = Afero.RemoveAll(p)
					if err == nil {
						bytes += tempFile.Size()
						counter++
					}
				}
			}
		}

		// Cleanup cache files
		cacheDir, err := os.UserCacheDir()
		if err == nil {
			scraperCacheDir := filepath.Join(cacheDir, AppName)
			if exists, err := Afero.Exists(scraperCacheDir); err == nil && exists {
				files, err := Afero.ReadDir(scraperCacheDir)
				if err == nil {
					counter += len(files)
					for _, f := range files {
						bytes += f.Size()
					}
				}

				_ = Afero.RemoveAll(scraperCacheDir)
			}
		}

		fmt.Printf("\U0001F9F9 %d files removed. Cleaned up %.2fMB\n", counter, BytesToMegabytes(bytes))
	},
}

var whereCmd = &cobra.Command{
	Use:   "where",
	Short: "Show path where config is located",
	Long:  "Show path where config is located if exists.\nOtherwise show path where it is expected to be",
	Run: func(cmd *cobra.Command, args []string) {
		edit, err := cmd.Flags().GetBool("edit")

		if err != nil {
			log.Fatal("Unexpected error while getting flag")
		}

		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Can't get config location, permission denied, probably")
		}

		exists, err := Afero.Exists(configPath)

		if err != nil {
			log.Fatalf("Can't understand if config exists or not. It is expected at\n%s\n", configPath)
		}

		if exists {

			if edit {
				if err := open.Start(configPath); err != nil {
					log.Fatal("Can not open the editor")
				}

				return
			}

			fmt.Printf("Config exists at\n%s\n", configPath)
		} else {
			fmt.Printf("Config doesn't exist, but it is expected to be at\n%s\n", configPath)
		}
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config at default path",
	Long:  "Create default config at default path if it doesn't exist yet",
	Run: func(cmd *cobra.Command, args []string) {
		force, err := cmd.Flags().GetBool("force")

		if err != nil {
			log.Fatal("Unexpected error while getting flag")
		}

		configPath, err := GetConfigPath()

		if err != nil {
			log.Fatal("Can't get config location, permission denied, probably")
		}

		exists, err := Afero.Exists(configPath)

		var createConfig = func() {
			if err := Afero.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
				log.Fatal("Error while creating file")
			} else if file, err := Afero.Create(configPath); err != nil {
				log.Fatal("Error while creating file")
			} else if _, err = file.Write(DefaultConfigBytes); err != nil {
				log.Fatal("Error while writing to file")
			} else {
				fmt.Printf("Config created at\n%s\n", configPath)
			}
		}

		if err != nil {
			if force {
				createConfig()
				return
			}

			log.Fatalf("Can't understand if config exists or not. It is expected at\n%s\n", configPath)
		}

		if exists {
			if force {
				createConfig()
				return
			}

			log.Fatal("Config file already exists. Use --force to overwrite it")
		} else {
			createConfig()
		}
	},
}

func CmdExecute() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(cleanupCmd)

	initCmd.Flags().BoolP("force", "f", false, "overwrite existing config")
	rootCmd.AddCommand(initCmd)

	whereCmd.Flags().BoolP("edit", "e", false, "open in the editor")
	rootCmd.AddCommand(whereCmd)

	rootCmd.Flags().StringP("config", "c", "", "use config from path")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}