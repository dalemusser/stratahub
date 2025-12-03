package main

import (
	"context"
	"log"

	"github.com/dalemusser/waffle/app"
	"github.com/dalemusser/stratahub/internal/app/bootstrap"
)

func main() {
	if err := app.Run(context.Background(), bootstrap.Hooks); err != nil {
		log.Fatal(err)
	}
}
