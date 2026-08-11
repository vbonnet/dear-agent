package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	maxWorkflowYAMLDepth = 128
	maxWorkflowYAMLNodes = 20000
)

func parseWorkflowYAML(blob []byte) (*yaml.Node, string, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(blob))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, "", err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple workflow YAML documents")
		}
		return nil, "", err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, "", errors.New("workflow YAML root is not one mapping")
	}
	root := document.Content[0]
	if err := validateWorkflowYAML(root); err != nil {
		return nil, "", err
	}
	var canonical strings.Builder
	appendCanonicalWorkflowNode(&canonical, root)
	return root, canonical.String(), nil
}

func appendCanonicalWorkflowNode(out *strings.Builder, node *yaml.Node) {
	appendCanonicalWorkflowNodeAt(out, node, workflowCanonicalRoot)
}

type workflowCanonicalLocation uint8

type workflowCanonicalPair struct {
	key      string
	rawKey   string
	value    *yaml.Node
	location workflowCanonicalLocation
}

const (
	workflowCanonicalOther workflowCanonicalLocation = iota
	workflowCanonicalRoot
	workflowCanonicalJobs
	workflowCanonicalJob
	workflowCanonicalStrategy
	workflowCanonicalMatrix
)

func appendCanonicalWorkflowNodeAt(out *strings.Builder, node *yaml.Node, location workflowCanonicalLocation) {
	switch node.Kind {
	case yaml.MappingNode:
		pairs := make([]workflowCanonicalPair, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			rawKey := node.Content[i].Value
			pairs = append(pairs, workflowCanonicalPair{
				key:      node.Content[i].Tag + ":" + rawKey,
				rawKey:   rawKey,
				value:    node.Content[i+1],
				location: childWorkflowCanonicalLocation(location, rawKey),
			})
		}
		// GitHub uses matrix variable insertion order to determine job creation
		// order. Preserve that one provider-semantic mapping order while
		// normalizing display-only order everywhere else.
		if location == workflowCanonicalMatrix {
			pairs = canonicalWorkflowMatrixPairs(pairs)
		} else {
			sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
		}
		out.WriteString("map{")
		for _, item := range pairs {
			fmt.Fprintf(out, "%q=", item.key)
			appendCanonicalWorkflowNodeAt(out, item.value, item.location)
			out.WriteByte(';')
		}
		out.WriteByte('}')
	case yaml.SequenceNode:
		out.WriteString("seq[")
		for _, child := range node.Content {
			appendCanonicalWorkflowNodeAt(out, child, workflowCanonicalOther)
			out.WriteByte(';')
		}
		out.WriteByte(']')
	case yaml.ScalarNode:
		fmt.Fprintf(out, "scalar(%q,%q)", node.Tag, node.Value)
	case yaml.DocumentNode, yaml.AliasNode:
		fmt.Fprintf(out, "node(%d)", node.Kind)
	default:
		fmt.Fprintf(out, "node(%d)", node.Kind)
	}
}

func canonicalWorkflowMatrixPairs(pairs []workflowCanonicalPair) []workflowCanonicalPair {
	axes := make([]workflowCanonicalPair, 0, len(pairs))
	reserved := make([]workflowCanonicalPair, 0, 2)
	for _, item := range pairs {
		switch item.rawKey {
		case "include", "exclude":
			reserved = append(reserved, item)
		default:
			axes = append(axes, item)
		}
	}
	sort.Slice(reserved, func(i, j int) bool { return reserved[i].key < reserved[j].key })
	return append(axes, reserved...)
}

func childWorkflowCanonicalLocation(parent workflowCanonicalLocation, key string) workflowCanonicalLocation {
	switch parent {
	case workflowCanonicalRoot:
		if key == "jobs" {
			return workflowCanonicalJobs
		}
	case workflowCanonicalJobs:
		return workflowCanonicalJob
	case workflowCanonicalJob:
		if key == "strategy" {
			return workflowCanonicalStrategy
		}
	case workflowCanonicalStrategy:
		if key == "matrix" {
			return workflowCanonicalMatrix
		}
	case workflowCanonicalOther, workflowCanonicalMatrix:
		return workflowCanonicalOther
	}
	return workflowCanonicalOther
}

func validateWorkflowYAML(node *yaml.Node) error {
	count := 0
	return validateWorkflowYAMLNode(node, 0, &count)
}

func validateWorkflowYAMLNode(node *yaml.Node, depth int, count *int) error {
	(*count)++
	if depth > maxWorkflowYAMLDepth || *count > maxWorkflowYAMLNodes {
		return errors.New("workflow YAML exceeds structural review bounds")
	}
	if err := validateWorkflowYAMLNodeShape(node); err != nil {
		return err
	}
	for _, child := range node.Content {
		if err := validateWorkflowYAMLNode(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowYAMLNodeShape(node *yaml.Node) error {
	if node == nil || node.Kind == yaml.AliasNode || node.Anchor != "" ||
		strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") {
		return errors.New("workflow YAML contains aliases, anchors, or custom tags")
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return validateWorkflowYAMLMapping(node)
}

func validateWorkflowYAMLMapping(node *yaml.Node) error {
	if len(node.Content)%2 != 0 {
		return errors.New("workflow YAML mapping is malformed")
	}
	seen := make(map[string]bool, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		// GitHub Actions constant-folds expression-only mapping keys before
		// schema evaluation. Reject them rather than letting a computed
		// permissions, environment, credentials, event, or matrix key bypass
		// the static authority classifier.
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "" || key.Value == "<<" ||
			strings.Contains(key.Value, "${{") || seen[key.Value] {
			return errors.New("workflow YAML mapping key is ambiguous")
		}
		seen[key.Value] = true
	}
	return nil
}

func mappingNodeValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}
