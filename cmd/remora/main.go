package main

import (
	"fmt"
	"os"
)

// Version is set during build time
var Version = "dev"

func main() {
	fmt.Printf("Remora v%s\n", Version)
	fmt.Println("GitHub reminder bot")
	os.Exit(0)
}
