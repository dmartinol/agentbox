package tui

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
