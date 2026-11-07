package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	pageDashboard = "dashboard"
	pageRuns      = "runs"
	pageImages    = "images"
	pageGateways  = "gateways"
	pagePolicies  = "policies"
	pageProfiles  = "profiles"
	pageDoctor    = "doctor"
)

var sectionPages = []string{
	pageDashboard,
	pageRuns,
	pageImages,
	pageGateways,
	pagePolicies,
	pageProfiles,
	pageDoctor,
}

type UI struct {
	ctx          context.Context
	app          *app.App
	tapp         *tview.Application
	pages        *tview.Pages
	navigation   *tview.List
	header       *tview.TextView
	footer       *tview.TextView
	current      string
	history      navigationHistory
	dynamicPages map[string]tview.Primitive
	modalActive  bool
}

// Run starts the AgentBox terminal user interface.
func Run(ctx context.Context, a *app.App) error {
	if a == nil {
		return fmt.Errorf("application is required")
	}
	ui := &UI{
		ctx:          ctx,
		app:          a,
		tapp:         tview.NewApplication(),
		dynamicPages: map[string]tview.Primitive{},
	}
	ui.buildLayout()
	ui.tapp.SetInputCapture(ui.handleGlobalKeys)
	return ui.tapp.SetRoot(ui.root(), true).EnableMouse(true).Run()
}

func (ui *UI) buildLayout() {
	ui.header = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[::b]AgentBox — OpenShell runner[::-]")
	ui.header.SetBorder(true).SetTitle(" AgentBox ")

	ui.navigation = tview.NewList().
		ShowSecondaryText(true).
		AddItem("Dashboard", "Overview", 'd', func() { ui.show(pageDashboard) }).
		AddItem("Runs", "Workloads", 'r', func() { ui.show(pageRuns) }).
		AddItem("Images", "Workloads", 'i', func() { ui.show(pageImages) }).
		AddItem("Gateways", "Infrastructure", 'g', func() { ui.show(pageGateways) }).
		AddItem("Policies", "Infrastructure", 'p', func() { ui.show(pagePolicies) }).
		AddItem("Profiles", "Configuration", 'o', func() { ui.show(pageProfiles) }).
		AddItem("Doctor", "Diagnostics", 'x', func() { ui.show(pageDoctor) })
	ui.navigation.SetBorder(true).SetTitle(" Navigation ")

	ui.footer = tview.NewTextView().
		SetDynamicColors(true).
		SetText("F1–F7 sections • Ctrl+[ / Alt+← back • Ctrl+] / Alt+→ forward • Tab fields • Enter select • q quit")
	ui.footer.SetBorder(true).SetTitle(" Help ")

	ui.pages = tview.NewPages()
	ui.render(pageDashboard)
}

func (ui *UI) handleGlobalKeys(event *tcell.EventKey) *tcell.EventKey {
	if index, ok := sectionForFunctionKey(event.Key()); ok {
		ui.switchSection(index)
		return nil
	}
	switch event.Key() {
	case tcell.KeyRune:
		if ui.modalActive {
			break
		}
		switch {
		case event.Rune() == 'q' && !isFormPage(ui.current):
			ui.tapp.Stop()
			return nil
		case ui.current == pageDashboard && event.Rune() == '1':
			ui.show("run-form")
			return nil
		case ui.current == pageDashboard && event.Rune() == '2':
			ui.show("image-build-form")
			return nil
		case ui.current == pageDashboard && event.Rune() == '3':
			ui.show("gateway-form")
			return nil
		}
	case tcell.KeyEscape:
		if !ui.modalActive {
			ui.back()
			return nil
		}
	case tcell.KeyCtrlLeftSq, tcell.KeyLeft:
		if event.Key() == tcell.KeyCtrlLeftSq || event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			ui.back()
			return nil
		}
	case tcell.KeyCtrlRightSq, tcell.KeyRight:
		if event.Key() == tcell.KeyCtrlRightSq || event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			ui.forward()
			return nil
		}
	}
	return event
}

func (ui *UI) switchSection(index int) {
	if index < 0 || index >= len(sectionPages) {
		return
	}
	ui.navigation.SetCurrentItem(index)
	ui.show(sectionPages[index])
}

func (ui *UI) root() tview.Primitive {
	body := tview.NewFlex().
		AddItem(ui.navigation, 24, 1, false).
		AddItem(ui.pages, 0, 8, true)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(ui.header, 3, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(ui.footer, 3, 0, false)
}

func (ui *UI) show(page string) {
	ui.history = pushNavigation(ui.history, ui.current, page)
	ui.render(page)
}

func (ui *UI) render(page string) {
	ui.current = page
	ui.pages.RemovePage(page)
	var primitive tview.Primitive
	switch page {
	case pageDashboard:
		primitive = ui.dashboardPage()
	case pageRuns:
		primitive = ui.runsPage()
	case pageImages:
		primitive = ui.imagesPage()
	case pageGateways:
		primitive = ui.gatewaysPage()
	case pagePolicies:
		primitive = ui.policiesPage()
	case pageProfiles:
		primitive = ui.profilesPage()
	case pageDoctor:
		primitive = ui.doctorPage()
	case "run-form":
		primitive = ui.runForm()
	case "image-version-form":
		primitive = ui.imageVersionForm()
	case "image-build-form":
		primitive = ui.imageBuildForm()
	case "image-push-form":
		primitive = ui.imagePushForm()
	case "image-pull-form":
		primitive = ui.imagePullForm()
	case "gateway-form":
		primitive = ui.gatewayForm()
	case "gateway-info-form":
		primitive = ui.gatewayInfoForm()
	case "provider-form":
		primitive = ui.providerForm()
	case "policy-host-form":
		primitive = ui.policyHostForm()
	case "profile-form":
		primitive = ui.profileForm()
	default:
		if dynamic, ok := ui.dynamicPages[page]; ok {
			primitive = dynamic
		} else {
			primitive = tview.NewTextView().SetText("Unknown page")
		}
	}
	ui.pages.AddAndSwitchToPage(page, primitive, true)
	ui.tapp.SetFocus(primitive)
	ui.header.SetText(ui.headerText())
	ui.footer.SetText(helpFor(page))
}

func (ui *UI) showDynamic(page string, primitive tview.Primitive) {
	ui.history = pushNavigation(ui.history, ui.current, page)
	ui.dynamicPages[page] = primitive
	ui.current = page
	ui.pages.AddAndSwitchToPage(page, primitive, true)
	ui.tapp.SetFocus(primitive)
	ui.header.SetText(ui.headerText())
	ui.footer.SetText(helpFor(page))
}

func (ui *UI) back() {
	next, page, ok := backNavigation(ui.history, ui.current)
	if !ok {
		return
	}
	ui.history = next
	ui.render(page)
}

func (ui *UI) forward() {
	next, page, ok := forwardNavigation(ui.history, ui.current)
	if !ok {
		return
	}
	ui.history = next
	ui.render(page)
}

func (ui *UI) headerText() string {
	back := " "
	if len(ui.history.past) > 0 {
		back = "‹"
	}
	forward := " "
	if len(ui.history.future) > 0 {
		forward = "›"
	}
	return fmt.Sprintf("[::b]%s %s %s[::-]", back, breadcrumb(ui.current), forward)
}

func breadcrumb(page string) string {
	parts := []string{"AgentBox"}
	if parent := parentSection(page); parent != "" {
		parts = append(parts, parent)
	}
	parts = append(parts, pageTitle(page))
	return strings.Join(parts, " / ")
}

func parentSection(page string) string {
	switch page {
	case "run-form":
		return "Runs"
	case "image-version-form", "image-build-form", "image-push-form", "image-pull-form":
		return "Images"
	case "gateway-form", "gateway-info-form":
		return "Gateways"
	case "provider-form", "profile-form":
		return "Profiles"
	case "policy-host-form":
		return "Policies"
	default:
		if strings.HasPrefix(page, "gateway-info:") {
			return "Gateways"
		}
		if strings.HasPrefix(page, "logs:") || strings.HasPrefix(page, "diff:") {
			return "Runs"
		}
		return ""
	}
}

func isFormPage(page string) bool {
	switch page {
	case "run-form", "image-version-form", "image-build-form", "image-push-form", "image-pull-form", "gateway-form", "gateway-info-form", "provider-form", "policy-host-form", "profile-form":
		return true
	default:
		return false
	}
}

func helpFor(page string) string {
	if strings.HasPrefix(page, "logs:") || strings.HasPrefix(page, "diff:") {
		return "Esc / Ctrl+[ / Alt+← back • Ctrl+] / Alt+→ forward • Tab focus • q quit"
	}
	switch page {
	case "run-form", "image-version-form", "image-build-form", "image-push-form", "image-pull-form", "gateway-form", "gateway-info-form", "provider-form", "policy-host-form", "profile-form":
		return "Tab next field • Shift+Tab previous • Enter submit • Ctrl+[ / Alt+← back • Ctrl+] / Alt+→ forward"
	case pageDashboard:
		return "1 Start run • 2 Build image • 3 Connect gateway • Tab actions • F1–F7 sections • Esc back • q quit"
	default:
		return "F1–F7 sections • Esc / Ctrl+[ / Alt+← back • Ctrl+] / Alt+→ forward • Tab focus • Enter select • q quit"
	}
}

func pageTitle(page string) string {
	switch page {
	case pageDashboard:
		return "Dashboard"
	case pageRuns:
		return "Runs"
	case pageImages:
		return "Images"
	case pageGateways:
		return "Gateways"
	case pagePolicies:
		return "Policies"
	case pageProfiles:
		return "Profiles"
	case pageDoctor:
		return "Doctor"
	case "run-form":
		return "Start run"
	case "image-version-form":
		return "Discover image version"
	case "image-build-form":
		return "Build image"
	case "image-push-form":
		return "Push image"
	case "image-pull-form":
		return "Pull image"
	case "gateway-form":
		return "Connect gateway"
	case "gateway-info-form":
		return "Inspect gateway"
	case "provider-form":
		return "Set up provider"
	case "policy-host-form":
		return "Allow network host"
	case "profile-form":
		return "Create profile"
	default:
		if strings.HasPrefix(page, "logs:") {
			return "Logs"
		}
		if strings.HasPrefix(page, "diff:") {
			return "Diff"
		}
		if strings.HasPrefix(page, "gateway-info:") {
			return "Gateway info"
		}
		return "Unknown"
	}
}

func (ui *UI) message(format string, args ...any) {
	ui.footer.SetText(fmt.Sprintf(format, args...))
}

func (ui *UI) refresh() {
	ui.render(ui.current)
}

func (ui *UI) async(action func() error) {
	ui.asyncTo(ui.current, action)
}

func (ui *UI) asyncTo(page string, action func() error) {
	go func() {
		err := action()
		ui.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ui.message("[red]Error: %s", err)
				return
			}
			ui.message("[green]Action completed.")
			ui.show(page)
		})
	}()
}

func (ui *UI) suspend(action func() error) {
	go func() {
		var err error
		ui.tapp.Suspend(func() {
			err = action()
		})
		ui.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ui.message("[red]Error: %s", err)
				return
			}
			ui.message("[green]Action completed.")
			ui.refresh()
		})
	}()
}
