package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultBrandName is the name of the platform unless an instance gives its own.
const DefaultBrandName = "Bingle"

// maxBrandIcon bounds an icon: it is read at start and kept in memory.
const maxBrandIcon = 512 * 1024

// A name goes into pages, mail headers and calendar files, so it is held to what is safe in all.
var brandNameRE = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} .'_-]{0,39}$`)

// brandIconTypes are the kinds of picture an icon can be.
var brandIconTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

// Brand is what an instance of the platform calls itself (ADR 0110).
type Brand struct {
	// Name is shown wherever a player sees the platform's name (BINGLE_BRAND_NAME).
	Name string
	// Icon is the picture (BINGLE_BRAND_ICON, a path), and IconType what kind it is. Nil is the default.
	Icon     []byte
	IconType string
	// DiscordURL is the link to this instance's Discord (BINGLE_DISCORD_URL), empty for none.
	DiscordURL string
}

// DisplayName is the name to show, the default if none was set.
func (b Brand) DisplayName() string {
	if b.Name == "" {
		return DefaultBrandName
	}
	return b.Name
}

// ParseDiscordURL checks the link to an instance's Discord: an https link to discord.gg or discord.com,
// since the button says Discord. Empty is none.
func ParseDiscordURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	host := ""
	if err == nil {
		host = strings.ToLower(u.Hostname())
	}
	ok := err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(host == "discord.gg" || host == "discord.com" || strings.HasSuffix(host, ".discord.com"))
	if !ok {
		return "", fmt.Errorf("BINGLE_DISCORD_URL %q: use an https link to discord.gg or discord.com, such as https://discord.gg/invite", raw)
	}
	return u.String(), nil
}

// LoadBrand reads the name and icon given. A mistake stops the start rather than showing up later.
func LoadBrand(name, iconPath string) (Brand, error) {
	var b Brand
	name = strings.TrimSpace(name)
	if name != "" {
		if !brandNameRE.MatchString(name) {
			return b, fmt.Errorf("BINGLE_BRAND_NAME %q: use up to 40 letters, digits, spaces and . ' _ -, starting with a letter or digit", name)
		}
		b.Name = name
	}
	if iconPath == "" {
		return b, nil
	}
	kind, ok := brandIconTypes[strings.ToLower(filepath.Ext(iconPath))]
	if !ok {
		return b, fmt.Errorf("BINGLE_BRAND_ICON %q: use an .svg, .png, .webp, .ico or .jpg file", iconPath)
	}
	data, err := os.ReadFile(iconPath)
	if err != nil {
		return b, fmt.Errorf("BINGLE_BRAND_ICON: %w", err)
	}
	if len(data) == 0 || len(data) > maxBrandIcon {
		return b, fmt.Errorf("BINGLE_BRAND_ICON %q: the file must be between 1 byte and %d KB", iconPath, maxBrandIcon/1024)
	}
	// A file named for one kind of picture must be it: a browser would otherwise show nothing.
	if kind != "image/svg+xml" && kind != "image/x-icon" && http.DetectContentType(data) != kind {
		return b, fmt.Errorf("BINGLE_BRAND_ICON %q: the file is not a %s", iconPath, kind)
	}
	if kind == "image/svg+xml" && !strings.Contains(string(data), "<svg") {
		return b, fmt.Errorf("BINGLE_BRAND_ICON %q: the file is not an SVG", iconPath)
	}
	b.Icon, b.IconType = data, kind
	return b, nil
}
