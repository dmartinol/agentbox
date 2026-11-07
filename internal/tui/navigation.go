package tui

import "github.com/gdamore/tcell/v2"

type navigationHistory struct {
	past   []string
	future []string
}

func pushNavigation(history navigationHistory, current, next string) navigationHistory {
	if current == next || current == "" {
		return history
	}
	past := append([]string{}, history.past...)
	if len(past) >= 50 {
		past = past[1:]
	}
	return navigationHistory{
		past:   append(past, current),
		future: nil,
	}
}

func backNavigation(history navigationHistory, current string) (navigationHistory, string, bool) {
	if len(history.past) == 0 {
		return history, current, false
	}
	previous := history.past[len(history.past)-1]
	past := history.past[:len(history.past)-1]
	future := append([]string{current}, history.future...)
	if len(future) > 50 {
		future = future[:50]
	}
	return navigationHistory{past: past, future: future}, previous, true
}

func forwardNavigation(history navigationHistory, current string) (navigationHistory, string, bool) {
	if len(history.future) == 0 {
		return history, current, false
	}
	next := history.future[0]
	future := history.future[1:]
	past := append([]string{}, history.past...)
	if len(past) >= 50 {
		past = past[1:]
	}
	return navigationHistory{past: append(past, current), future: future}, next, true
}

func sectionForFunctionKey(key tcell.Key) (int, bool) {
	switch key {
	case tcell.KeyF1:
		return 0, true
	case tcell.KeyF2:
		return 1, true
	case tcell.KeyF3:
		return 2, true
	case tcell.KeyF4:
		return 3, true
	case tcell.KeyF5:
		return 4, true
	case tcell.KeyF6:
		return 5, true
	case tcell.KeyF7:
		return 6, true
	default:
		return 0, false
	}
}

func nextSection(current string, delta int) string {
	currentIndex := 0
	for i, page := range sectionPages {
		if page == current {
			currentIndex = i
			break
		}
	}
	nextIndex := (currentIndex + delta + len(sectionPages)) % len(sectionPages)
	return sectionPages[nextIndex]
}
