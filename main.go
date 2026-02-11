package main

import (
	"fmt"
	"os"

	"gitlab.com/gomidi/midi/v2"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

func main() {
	defer midi.CloseDriver()

	inPorts := midi.GetInPorts()
	outPorts := midi.GetOutPorts()

	fmt.Println("MIDI Input Ports:")
	if len(inPorts) == 0 {
		fmt.Println("  (none)")
	}
	for i, port := range inPorts {
		fmt.Printf("  [%d] %s\n", i, port)
	}

	fmt.Println("\nMIDI Output Ports:")
	if len(outPorts) == 0 {
		fmt.Println("  (none)")
	}
	for i, port := range outPorts {
		fmt.Printf("  [%d] %s\n", i, port)
	}

	if len(inPorts) == 0 && len(outPorts) == 0 {
		fmt.Println("\nNo MIDI devices found.")
		os.Exit(1)
	}
}
