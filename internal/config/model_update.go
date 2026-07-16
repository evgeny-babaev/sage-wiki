package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ModelStageUpdate contains narrow, safe changes for one compiler stage.
type ModelStageUpdate struct {
	Model       string
	ExtraParams map[string]string
}

// UpdateModelStages edits only the models section of the raw YAML. It avoids
// Load+Save because Load expands environment placeholders, including API keys.
func UpdateModelStages(path string, updates map[string]ModelStageUpdate) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config.UpdateModelStages: read: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("config.UpdateModelStages: parse: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config.UpdateModelStages: root must be a mapping")
	}

	models := ensureMappingValue(doc.Content[0], "models")
	params := ensureMappingValue(models, "params")
	for stage, update := range updates {
		if update.Model != "" {
			setScalarValue(models, stage, update.Model)
		}
		if len(update.ExtraParams) == 0 {
			continue
		}
		stageParams := ensureMappingValue(params, stage)
		for key, value := range update.ExtraParams {
			setScalarValue(stageParams, key, value)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config.UpdateModelStages: stat: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.yaml-*.tmp")
	if err != nil {
		return fmt.Errorf("config.UpdateModelStages: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return fmt.Errorf("config.UpdateModelStages: chmod: %w", err)
	}
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		enc.Close()
		tmp.Close()
		return fmt.Errorf("config.UpdateModelStages: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		return fmt.Errorf("config.UpdateModelStages: close encoder: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("config.UpdateModelStages: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config.UpdateModelStages: close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("config.UpdateModelStages: replace: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config.UpdateModelStages: replace: %w", err)
	}
	return nil
}

func ensureMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			if value.Kind != yaml.MappingNode {
				value.Kind = yaml.MappingNode
				value.Tag = "!!map"
				value.Value = ""
				value.Content = nil
			}
			return value
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

func setScalarValue(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
