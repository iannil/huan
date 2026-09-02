module github.com/iannil/huan-plugin-ebook-exporter

go 1.26

replace github.com/iannil/huan => ../../

require gopkg.in/yaml.v3 v3.0.1

require (
	github.com/go-shiori/go-epub v1.2.1
	github.com/gpdf-dev/gpdf v1.0.11
	github.com/mmonterroca/docxgo/v2 v2.14.0
	github.com/yuin/goldmark v1.8.2
)

require (
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gofrs/uuid/v5 v5.0.0 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	golang.org/x/net v0.19.0 // indirect
)
