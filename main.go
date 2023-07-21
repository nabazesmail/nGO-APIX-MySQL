// main.go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nabazesmail/gopher/src/migrate"
	"github.com/nabazesmail/gopher/src/router"
)

// Run the migration logic
func init() {
	migrate.Migration()
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Error getting current working directory:", err)
	}
	fmt.Println("Current working directory:", cwd)

	r := router.SetupRouter()
	r.Run()
}
