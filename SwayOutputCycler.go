package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/joshuarubin/go-sway"
)

type Monitor struct {
	serial                 string
	position               [2]int
	resolution             [2]int
	refreshRate            float32
	variableRefreshEnabled bool
}

type Layout struct {
	name     string
	monitors []Monitor
}

func main() {
	layouts := []Layout{
		{
			"dual",
			[]Monitor{
				{
					serial:                 "910NTGYBT391",
					position:               [2]int{2560, 0},
					resolution:             [2]int{2560, 1440},
					refreshRate:            144,
					variableRefreshEnabled: false,
				},
				{
					serial:                 "VOKNAHA010346",
					position:               [2]int{0, 0},
					resolution:             [2]int{2560, 1440},
					refreshRate:            59.951000,
					variableRefreshEnabled: false,
				},
			},
		},
		{
			"single",
			[]Monitor{
				{
					serial:                 "910NTGYBT391",
					position:               [2]int{0, 0},
					resolution:             [2]int{2560, 1440},
					refreshRate:            144,
					variableRefreshEnabled: false,
				},
			},
		},
		{
			"beamer",
			[]Monitor{
				{
					serial:                 "0x00000001",
					position:               [2]int{0, 0},
					resolution:             [2]int{1920, 1080},
					refreshRate:            60,
					variableRefreshEnabled: false,
				},
			},
		},
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	stateFilePath, err := xdg.StateFile("mmp/activelayout")
	if err != nil {
		stateFilePath = homeDir + ".local/state/mmp/activelayout"
		log.Printf("Failed to detect XDG state dir, using %v", stateFilePath)
	}
	stateFileDir := filepath.Dir(stateFilePath)
	err = os.MkdirAll(stateFileDir, 0o644)
	if err != nil {
		log.Fatalf("Could not create state dir at %v. Error %v", stateFileDir, err)
	}
	activeLayoutBytes, err := os.ReadFile(stateFilePath)
	activeLayoutIdx := -1
	if err == nil {
		activeLayoutName := strings.TrimSpace(string(activeLayoutBytes))
		log.Printf("read layout %v from state file", activeLayoutName)
		for idx, layout := range layouts {
			if activeLayoutName == layout.name {
				activeLayoutIdx = idx
				break
			}
		}
	}
	activeLayoutIdx = (activeLayoutIdx + 1) % len(layouts)
	err = os.WriteFile(stateFilePath, []byte(layouts[activeLayoutIdx].name), 0o644)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Switching to layout %v", layouts[activeLayoutIdx].name)

	ctx := context.Background()
	client, err := sway.New(ctx)
	if err != nil {
		log.Fatal("Could not initialize sway client")
	}
	outputs, err := client.GetOutputs(ctx)
	if err != nil {
		log.Fatal("Could not get sway outputs")
	}

	activeLayout := layouts[activeLayoutIdx]
	commands := make([]string, 0, len(activeLayout.monitors)+len(outputs))
	outputIds := make([]string, len(outputs))
	for i := range outputIds {
		output := outputs[i]
		outputIds[i] = fmt.Sprintf("%v %v %v", output.Make, output.Model, output.Serial)
	}
	outputsEnabled := make([]bool, len(outputs))
	for i := range outputsEnabled {
		outputsEnabled[i] = false
	}
	for _, monitorConfig := range activeLayout.monitors {
		monitorId := ""
		for outIdx, output := range outputs {
			if output.Serial == monitorConfig.serial {
				monitorId = outputIds[outIdx]
				outputsEnabled[outIdx] = true
				break
			}
		}
		if monitorId == "" {
			continue
		}
		command := fmt.Sprintf("output \"%v\" res %vx%v pos %v %v",
			monitorId,
			monitorConfig.resolution[0],
			monitorConfig.resolution[1],
			monitorConfig.position[0],
			monitorConfig.position[1],
		)
		commands = append(commands, command)
	}
	for i, outputId := range outputIds {
		var state string
		if outputsEnabled[i] {
			state = "enable"
		} else {
			state = "disable"
		}
		commands = append(commands, fmt.Sprintf("output \"%v\" %v", outputId, state))
	}
	command := strings.Join(commands, ";")
	log.Println(command)
	_, err = client.RunCommand(ctx, command)
	if err != nil {
		log.Fatal(err)
	}
}
