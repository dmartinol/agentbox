package tui

import "testing"

func TestJSONTreeBuildsStructuredNodes(t *testing.T) {
	tree, err := jsonTree([]byte(`{"name":"openshell","active":true,"drivers":[{"name":"podman"}]}`), "Gateway info")
	if err != nil {
		t.Fatal(err)
	}
	root := tree.GetRoot()
	if root.GetText() != "Gateway info" {
		t.Fatalf("root = %q", root.GetText())
	}
	if root.GetChildren() == nil || len(root.GetChildren()) != 3 {
		t.Fatalf("unexpected root children: %v", root.GetChildren())
	}
	foundName, foundDrivers := false, false
	for _, child := range root.GetChildren() {
		switch child.GetText() {
		case "name":
			if len(child.GetChildren()) == 1 && child.GetChildren()[0].GetText() == "openshell" {
				foundName = true
			}
		case "drivers":
			if len(child.GetChildren()) == 1 {
				drivers := child.GetChildren()[0]
				if len(drivers.GetChildren()) == 1 && drivers.GetChildren()[0].GetText() == "name" {
					foundDrivers = true
				}
			}
		}
	}
	if !foundName || !foundDrivers {
		t.Fatalf("structured nodes not found: name=%v drivers=%v", foundName, foundDrivers)
	}
}
