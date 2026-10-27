package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Publication contains optional publisher-owned copy and artwork. Zero values
// intentionally carry no identity. Languages are exact edition keys (zh/en).
type Publication struct {
	BaseURL         string                       `yaml:"base_url"`
	CoverBackground string                       `yaml:"cover_background"`
	Languages       map[Lang]PublicationIdentity `yaml:"languages"`
	BackgroundPNG   []byte                       `yaml:"-"`
}

// PublicationIdentity supplies literal copy, without inferred names or rights.
type PublicationIdentity struct {
	Author          string   `yaml:"author"`
	MetadataAuthor  string   `yaml:"metadata_author"`
	TitleCredit     string   `yaml:"title_credit"`
	Publisher       string   `yaml:"publisher"`
	Copyright       string   `yaml:"copyright"`
	Rights          string   `yaml:"rights"`
	CoverBrand      string   `yaml:"cover_brand"`
	CoverBrandLatin string   `yaml:"cover_brand_latin"`
	CoverTagline    []string `yaml:"cover_tagline"`
	CoverCredit     string   `yaml:"cover_credit"`
	CoverFooter     string   `yaml:"cover_footer"`
}

func (p Publication) Identity(lang Lang) PublicationIdentity { return p.Languages[lang] }

// Fingerprint includes the loaded artwork bytes, not merely its filename.
func (p Publication) Fingerprint() string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
