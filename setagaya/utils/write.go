package utils

import (
	"log"
	"os"
)

func MakeFolder(folderPath string) {
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		if err := os.MkdirAll(folderPath, 0750); err != nil { // More secure directory permissions
			log.Printf("Failed to create folder %s: %v", folderPath, err)
		}
	}
}

func DeleteFolder(folderPath string) {
	if err := os.RemoveAll(folderPath); err != nil {
		log.Printf("Failed to delete folder %s: %v", folderPath, err)
	}
}
