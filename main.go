package main

import (
	_ "embed"
	"os"

	"github.com/samcro1967/glance/internal/glance"
)

//go:embed README.md
var readme []byte

//go:embed CONTRIBUTING.md
var contributing []byte

func main() {
	os.Exit(glance.MainWithRootDocs(readme, contributing))
}
