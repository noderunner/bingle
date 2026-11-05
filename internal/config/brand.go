package config

import (
	"fmt"
	"net/http"
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
}

// DisplayName is the name to show, the default if none was set.
func (b Brand) DisplayName() string {
	if b.Name == "" {
		return DefaultBrandName
	}
	return b.Name
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
