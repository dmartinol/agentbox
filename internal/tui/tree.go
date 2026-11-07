package tui

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rivo/tview"
)

func jsonTree(data []byte, title string) (*tview.TreeView, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	root := tview.NewTreeNode(title).SetReference(title)
	addJSONNode(root, value)
	tree := tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root).
		SetGraphics(true)
	return tree, nil
}

func addJSONNode(parent *tview.TreeNode, value any) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := tview.NewTreeNode(key).SetReference(key)
			parent.AddChild(child)
			addJSONNode(child, typed[key])
		}
	case []any:
		for i, item := range typed {
			child := tview.NewTreeNode(fmt.Sprintf("[%d]", i)).SetReference(i)
			parent.AddChild(child)
			addJSONNode(child, item)
		}
	case nil:
		parent.AddChild(tview.NewTreeNode("null").SetReference(nil))
	default:
		parent.AddChild(tview.NewTreeNode(fmt.Sprintf("%v", typed)).SetReference(typed))
	}
}
