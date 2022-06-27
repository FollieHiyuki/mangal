package main

import (
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strings"
)

// FormatType is type of format used for output
type FormatType string

const (
	PDF   FormatType = "pdf"
	CBZ   FormatType = "cbz"
	Zip   FormatType = "zip"
	Plain FormatType = "plain"
	Epub  FormatType = "epub"
)

type UI struct {
	Fullscreen          bool
	Prompt              string
	Title               string
	Placeholder         string
	Mark                string
	ChapterNameTemplate string `toml:"chapter_name_template"`
}

type Config struct {
	Scrapers []*Scraper
	Format   FormatType
	UI       UI
	Anilist  struct {
		Client         *AnilistClient
		Enabled        bool
		MarkDownloaded bool
	}
	UseCustomReader     bool
	CustomReader        string
	Path                string
	CacheImages         bool
	ChapterNameTemplate string
}

// GetConfigPath returns path to config file
func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()

	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, strings.ToLower(Mangal), "config.toml"), nil
}

// DefaultConfig makes default config
func DefaultConfig() *Config {
	conf, _ := ParseConfig(DefaultConfigBytes)
	return conf
}

// UserConfig is a global variable that stores user config
var UserConfig *Config

// DefaultConfigBytes is default config in TOML format
var DefaultConfigBytes = []byte(`# Which sources to use. You can use several sources, it won't affect perfomance
use = ['manganelo']

# Type "mangal formats" to show more information about formats
format = "pdf"

# If false, then OS default reader will be used
use_custom_reader = false
custom_reader = "zathura"

# Custom download path, can be either relative (to the current directory) or absolute
download_path = '.'

# How chapters should be named when downloaded
# Use %d to specify chapter number and %s to specify chapter title
# If you want to pad chapter number with zeros for natural sorting (e.g. 0001, 0123) use %0d instead of %d
chapter_name_template = "[%0d] %s"

# Add images to cache
# If set to true mangal could crash when trying to redownload something quickly
# Usually happens on slow machines
cache_images = false

[anilist]
# Enable Anilist integration (BETA)
# See https://github.com/metafates/mangal/wiki/Anilist-Integration for more information
enabled = false

# Anilist client ID
id = ""

# Anilist client secret
secret = ""

# Will mark downloaded chapters as read on Anilist
mark_downloaded = false

[ui]
# How to display chapters in TUI mode
# Use %d to specify chapter number and %s to specify chapter title
chapter_name_template = "[%d] %s"

# Fullscreen mode 
fullscreen = true

# Input prompt symbol
prompt = ">"

# Input placeholder
placeholder = "What shall we look for?"

# Selected chapter mark
mark = "▼"

# Search window title
title = "Mangal"

[sources]
[sources.manganelo]
# Base url
base = 'https://m.manganelo.com'

# Chapters Base url
chapters_base = 'https://chap.manganelo.com/'

# Search endpoint. Put %s where the query should be
search = 'https://m.manganelo.com/search/story/%s'

# Selector of entry anchor (<a></a>) on search page
manga_anchor = '.search-story-item a.item-title'

# Selector of entry title on search page
manga_title = '.search-story-item a.item-title'

# Manga chapters anchors selector
chapter_anchor = 'li.a-h a.chapter-name'

# Manga chapters titles selector
chapter_title = 'li.a-h a.chapter-name'

# Reader page images selector
reader_page = '.container-chapter-reader img'

# Random delay between requests
random_delay_ms = 500 # ms

# Are chapters listed in reversed order on that source?
# reversed order -> from newest chapter to oldest
reversed_chapters_order = true

# With what character should the whitespace in query be replaced?
whitespace_escape = "_"
`)

// GetConfig returns user config or default config if it doesn't exist
// If path is empty string then default config will be returned
func GetConfig(path string) *Config {
	var (
		configPath string
		err        error
	)

	// If path is empty string then default config will be used
	if path == "" {
		configPath, err = GetConfigPath()
	} else {
		configPath = path
	}

	if err != nil {
		return DefaultConfig()
	}

	// If config file doesn't exist then default config will be used
	configExists, err := Afero.Exists(configPath)
	if err != nil || !configExists {
		return DefaultConfig()
	}

	// Read config file
	contents, err := Afero.ReadFile(configPath)
	if err != nil {
		return DefaultConfig()
	}

	// Parse config
	config, err := ParseConfig(contents)
	if err != nil {
		return DefaultConfig()
	}

	return config
}

// ParseConfig parses config from given string
func ParseConfig(configString []byte) (*Config, error) {
	// tempConfig is a temporary config that will be used to store parsed config
	type tempConfig struct {
		Use                 []string
		Format              string
		UI                  UI     `toml:"ui"`
		UseCustomReader     bool   `toml:"use_custom_reader"`
		CustomReader        string `toml:"custom_reader"`
		Path                string `toml:"download_path"`
		CacheImages         bool   `toml:"cache_images"`
		Sources             map[string]Source
		ChapterNameTemplate string `toml:"chapter_name_template"`
		Anilist             struct {
			Enabled        bool   `toml:"enabled"`
			ID             string `toml:"id"`
			Secret         string `toml:"secret"`
			MarkDownloaded bool   `toml:"mark_downloaded"`
		}
	}

	var (
		tempConf tempConfig
		conf     Config
	)
	err := toml.Unmarshal(configString, &tempConf)

	if err != nil {
		return nil, err
	}

	conf.CacheImages = tempConf.CacheImages

	// Convert sources listed in tempConfig to Scrapers
	for sourceName, source := range tempConf.Sources {
		// If source is not listed in Use then skip it
		if !Contains[string](tempConf.Use, sourceName) {
			continue
		}

		// Create scraper
		source.Name = sourceName
		scraper := MakeSourceScraper(&source)

		if !conf.CacheImages {
			scraper.FilesCollector.CacheDir = ""
		}

		conf.Scrapers = append(conf.Scrapers, scraper)
	}

	if tempConf.UI.ChapterNameTemplate == "" {
		tempConf.UI.ChapterNameTemplate = "[%d] %s"
	}

	conf.UI = tempConf.UI

	conf.Path = tempConf.Path

	// Default format is pdf
	conf.Format = IfElse(tempConf.Format == "", PDF, FormatType(tempConf.Format))

	conf.UseCustomReader = tempConf.UseCustomReader
	conf.CustomReader = tempConf.CustomReader

	if template := tempConf.ChapterNameTemplate; template != "" {
		conf.ChapterNameTemplate = template
	} else {
		conf.ChapterNameTemplate = "[%0d] %s"
	}

	if tempConf.Anilist.Enabled {
		id, secret := tempConf.Anilist.ID, tempConf.Anilist.Secret
		conf.Anilist.Client, err = NewAnilistClient(id, secret)

		if err != nil {
			return nil, err
		}

		conf.Anilist.Enabled = true
		conf.Anilist.MarkDownloaded = tempConf.Anilist.MarkDownloaded
	}

	return &conf, err
}

// ValidateConfig checks if config is valid and returns error if it is not
func ValidateConfig(config *Config) error {
	// check if any source is used
	if len(config.Scrapers) == 0 {
		return errors.New("no manga sources listed")
	}

	// check if chapter name template is valid
	// chapter name template should contain %d or %s placeholder
	validateChapterNameTemplate := func(template string) error {
		if !strings.Contains(template, "%d") &&
			!strings.Contains(template, "%s") &&
			!strings.Contains(template, "%0d") {
			return errors.New("chapter name template should contain at least one %d, %0d or %s placeholder")
		}
		return nil
	}

	err := validateChapterNameTemplate(config.ChapterNameTemplate)
	if err != nil {
		return err
	}

	err = validateChapterNameTemplate(config.UI.ChapterNameTemplate)
	if err != nil {
		return err
	}

	// check if anilist is properly configured
	if config.Anilist.Enabled {
		if config.Anilist.Client.ID == "" || config.Anilist.Client.Secret == "" {
			return errors.New("anilist is enabled but id or secret is not set")
		}
	}

	// check if custom reader is properly configured
	if config.UseCustomReader && config.CustomReader == "" {
		return errors.New("use_custom_reader is set to true but reader isn't specified")
	}

	// Check if format is valid
	if !Contains(AvailableFormats, config.Format) {
		msg := fmt.Sprintf(
			`unknown format '%s'
type %s to show available formats`,
			string(config.Format),
			accentStyle.Render(strings.ToLower(Mangal)+" formats"),
		)
		return errors.New(msg)
	}

	// Check if scrapers are valid
	for _, scraper := range config.Scrapers {
		if scraper.Source == nil {
			return errors.New("internal error: scraper source is nil")
		}
		if err := ValidateSource(scraper.Source); err != nil {
			return err
		}
	}

	return nil
}
