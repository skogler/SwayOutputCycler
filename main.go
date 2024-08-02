package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/joshuarubin/go-sway"
	"gopkg.in/yaml.v3"
)

type Monitor struct {
	Serial       string
	Position     [2]int `yaml:",flow"`
	Resolution   [2]int `yaml:",flow"`
	RefreshRate  float32
	AdaptiveSync bool
}

type Layout struct {
	Name     string
	Monitors []Monitor
}

type Config struct {
	Layouts []Layout
}

var config Config = Config{
	Layouts: []Layout{
		{
			"dual",
			[]Monitor{
				{
					Serial:       "910NTGYBT391",
					Position:     [2]int{2560, 0},
					Resolution:   [2]int{2560, 1440},
					RefreshRate:  120,
					AdaptiveSync: false,
				},
				{
					Serial:       "VOKNAHA010346",
					Position:     [2]int{0, 0},
					Resolution:   [2]int{2560, 1440},
					RefreshRate:  59.951000,
					AdaptiveSync: false,
				},
			},
		},
		{
			"single",
			[]Monitor{
				{
					Serial:       "910NTGYBT391",
					Position:     [2]int{0, 0},
					Resolution:   [2]int{2560, 1440},
					RefreshRate:  120,
					AdaptiveSync: false,
				},
			},
		},
		{
			"beamer",
			[]Monitor{
				{
					Serial:       "0x00000001",
					Position:     [2]int{0, 0},
					Resolution:   [2]int{1920, 1080},
					RefreshRate:  60,
					AdaptiveSync: false,
				},
			},
		},
	},
}

func printUsage() {
	print("Usage: SwayOutputCycler [<layout name>]")
	print("If layout name is not specified, cycles to the next layout")
}

func getConfigFilePath() (string, error) {
	return xdg.ConfigFile("SwayOutputCycler.yaml")
}

func loadConfig() {
	configFilePath, err := getConfigFilePath()
	if err != nil {
		log.Fatal("Failed to find suitable config file path")
	}
	// _, err = yaml.DecodeFile(configFilePath, &config)
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("Could not read config file from %s", configFilePath)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Could not read config file from %s", configFilePath)
	}
}

func saveConfig() {
	configFilePath, err := getConfigFilePath()
	log.Printf("Writing config to %s", configFilePath)
	if err != nil {
		log.Fatal("Failed to find suitable config file path")
	}
	data, err := yaml.Marshal(&config)
	if err != nil {
		log.Fatalf("Could not serialize config to yaml: %s", err)
	}
	print(string(data))
	err = os.WriteFile(configFilePath, data, 0o600)
	if err != nil {
		log.Fatalf("Could not write config to file: error: %s path: %s", err, configFilePath)
	}
}

func doesConfigExist() bool {
	configFilePath, err := getConfigFilePath()
	if err != nil {
		log.Fatal("Failed to find suitable config file path")
	}
	_, err = os.Stat(configFilePath)
	if err == nil {
		return true
	} else if errors.Is(err, os.ErrNotExist) {
		return false
	}
	log.Fatalf("Could not stat config file. Error: %s Path: %s", err, configFilePath)
	return false
}

func main() {
	log.SetFlags(0)
	if !doesConfigExist() {
		saveConfig()
	}
	loadConfig()

	selectedLayoutIdx := -1
	if len(os.Args) > 1 {
		selectedLayout := os.Args[1]
		if selectedLayout != "" {
			for idx, layout := range config.Layouts {
				if layout.Name == selectedLayout {
					selectedLayoutIdx = idx
					break
				}
			}
			if selectedLayoutIdx == -1 {
				var sb strings.Builder
				sb.WriteString("Given layout '")
				sb.WriteString(selectedLayout)
				sb.WriteString("' not found in config. Available layouts are:\n")
				for _, layout := range config.Layouts {
					sb.WriteString("    ")
					sb.WriteString(layout.Name)
					sb.WriteString("\n")
				}
				log.Fatal(sb.String())
			}
		}
	}
	activeLayoutIdx := -1
	stateFilePath := initStateFile()
	activeLayoutIdx = loadLayoutStateFromFile(stateFilePath, config.Layouts, activeLayoutIdx)

	if selectedLayoutIdx != -1 {
		activeLayoutIdx = selectedLayoutIdx
	} else {
		activeLayoutIdx = cycleToNextLayout(config.Layouts, activeLayoutIdx)
	}

	saveLayoutState(stateFilePath, config.Layouts, activeLayoutIdx)
	applyLayout(activeLayoutIdx)
}

func applyLayout(activeLayoutIdx int) {
	ctx := context.Background()
	client, err := sway.New(ctx)
	if err != nil {
		log.Fatal("Could not initialize sway client")
	}
	outputs, err := client.GetOutputs(ctx)
	if err != nil {
		log.Fatal("Could not get sway outputs")
	}

	activeLayout := config.Layouts[activeLayoutIdx]
	commands := make([]string, 0, len(activeLayout.Monitors)+len(outputs))
	outputIds := make([]string, len(outputs))
	for i := range outputIds {
		output := outputs[i]
		outputIds[i] = fmt.Sprintf("%v %v %v", output.Make, output.Model, output.Serial)
	}
	outputsEnabled := make([]bool, len(outputs))
	for i := range outputsEnabled {
		outputsEnabled[i] = false
	}
	for _, monitorConfig := range activeLayout.Monitors {
		monitorId := ""
		for outIdx, output := range outputs {
			if output.Serial == monitorConfig.Serial {
				monitorId = outputIds[outIdx]
				outputsEnabled[outIdx] = true
				break
			}
		}
		if monitorId == "" {
			continue
		}
		command := fmt.Sprintf("output \"%v\" mode %vx%v@%vHz pos %v %v",
			monitorId,
			monitorConfig.Resolution[0],
			monitorConfig.Resolution[1],
			monitorConfig.RefreshRate,
			monitorConfig.Position[0],
			monitorConfig.Position[1],
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

func initStateFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	stateFilePath, err := xdg.StateFile("SwayOutputCycler/activelayout")
	if err != nil {
		stateFilePath = homeDir + ".local/state/SwayOutputCycler/activelayout"
		log.Printf("Failed to detect XDG state dir, using %v", stateFilePath)
	}
	stateFileDir := filepath.Dir(stateFilePath)
	err = os.MkdirAll(stateFileDir, 0o644)
	if err != nil {
		log.Fatalf("Could not create state dir at %v. Error %v", stateFileDir, err)
	}
	return stateFilePath
}

func saveLayoutState(stateFilePath string, layouts []Layout, activeLayoutIdx int) {
	err := os.WriteFile(stateFilePath, []byte(layouts[activeLayoutIdx].Name), 0o644)
	if err != nil {
		log.Fatal(err)
	}
}

func cycleToNextLayout(layouts []Layout, activeLayoutIdx int) int {
	activeLayoutIdx = (activeLayoutIdx + 1) % len(layouts)
	log.Printf("Switching to layout %v", layouts[activeLayoutIdx].Name)
	return activeLayoutIdx
}

func loadLayoutStateFromFile(stateFilePath string, layouts []Layout, activeLayoutIdx int) int {
	activeLayoutBytes, err := os.ReadFile(stateFilePath)
	if err == nil {
		activeLayoutName := strings.TrimSpace(string(activeLayoutBytes))
		log.Printf("read layout %v from state file", activeLayoutName)
		for idx, layout := range layouts {
			if activeLayoutName == layout.Name {
				activeLayoutIdx = idx
				break
			}
		}
	}
	return activeLayoutIdx
}
