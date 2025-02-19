package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"git.sr.ht/~relay/medusa"
	"git.sr.ht/~relay/medusa/transformers/collections"
	"git.sr.ht/~relay/medusa/transformers/layouts"
	"git.sr.ht/~relay/medusa/transformers/markdown"
	"git.sr.ht/~relay/medusa/transformers/metadata"
)

func main() {
	config := medusa.Config{
		Logger: slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
		AutoConfirm: true,
	}

	b := medusa.NewBuilder(config)
	b.Source("./src")
	b.Destination("./_build")

	b.Use(metadata.New(
		map[string]any{
			"title":       "Sapp",
			"description": "Sarah og Sebastians App!",
		},
	))

	b.Use(collections.New(collections.CollectionConfig{
		Name: "Server",
		Store: map[string]any{
			"URL": os.Getenv("SERVER_URL"),
		},
		Patterns: []string{"blog/*.md"},
	}))

	b.Use(func(files *[]medusa.File, store *medusa.Store) error {
		for i, file := range *files {
			if file.FileInfo.Name() == "tailwind.css" {
				fmt.Println(file.Path)
				output, err := exec.Command("tailwindcss", "--content", "./**/*.{html,js}", "-i", filepath.Join("src", file.Path)).Output()
				if err != nil {
					return err
				}
				(*files)[i].Path = filepath.Join(filepath.Dir(file.Path), "style.css")
				(*files)[i].SetContent(output)
			}
		}
		return nil
	})

	b.Use(markdown.New())

	b.Use(layouts.New(layouts.Config{
		LayoutPatterns:  []string{"templates/*"},
		ContentPatterns: []string{"*.html"},
	}))

	err := b.Build()
	if err != nil {
		panic(err)
	}
}
