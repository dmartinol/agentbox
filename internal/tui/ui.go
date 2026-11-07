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
	ctx            context.Context
	app            *app.App
	tapp           *tview.Application
	pages          *tview.Pages
	navigation     *tview.List
	header         *tview.TextView
	footer         *tview.TextView
	current        string
	history        navigationHistory
	dynamicPages   map[string]tview.Primitive
	modalActive    bool
	currentContent tview.Primitive
	currentActions *tview.Form
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
	ui.tapp.SetFocus(ui.navigation)
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
		SetText("Tab next layer • Shift+Tab previous • Esc back • Arrows move • Enter select • q quit")
	ui.footer.SetBorder(true).SetTitle(" Help ")

	ui.pages = tview.NewPages()
	ui.render(pageDashboard)
}

func (ui *UI) handleGlobalKeys(event *tcell.EventKey) *tcell.EventKey {
	if ui.modalActive {
		return event
	}

	switch event.Key() {
	case tcell.KeyRune:
		if event.Rune() == 'q' && !isFormPage(ui.current) {
			ui.tapp.Stop()
			return nil
		}
	case tcell.KeyTab:
		if ui.navigation.HasFocus() {
			ui.setFocusLayer(1)
			return nil
		}
		if ui.currentContent != nil && ui.currentContent.HasFocus() && ui.currentActions != nil && !ui.currentContentIsForm() {
			ui.setFocusLayer(2)
			return nil
		}
	case tcell.KeyBacktab:
		if ui.currentActions != nil && ui.currentActions.HasFocus() {
			ui.setFocusLayer(1)
			return nil
		}
		if ui.currentContent != nil && ui.currentContent.HasFocus() {
			ui.setFocusLayer(0)
			return nil
		}
	case tcell.KeyEscape:
		if ui.currentActions != nil && ui.currentActions.HasFocus() {
			ui.setFocusLayer(1)
			return nil
		}
		if isFormPage(ui.current) {
			ui.back()
			return nil
		}
		if ui.currentContent != nil && ui.currentContent.HasFocus() {
			ui.setFocusLayer(0)
			return nil
		}
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		if ui.currentActions != nil && ui.currentActions.HasFocus() {
			ui.moveActionFocus(event.Key())
			return nil
		}
	}
	return event
}

func (ui *UI) setFocusLayer(layer int) {
	switch layer {
	case 0:
		ui.tapp.SetFocus(ui.navigation)
	case 1:
		if ui.currentContent != nil {
			ui.tapp.SetFocus(ui.currentContent)
		}
	case 2:
		if ui.currentActions != nil {
			ui.tapp.SetFocus(ui.currentActions)
		}
	}
}

func (ui *UI) currentContentIsForm() bool {
	_, ok := ui.currentContent.(*tview.Form)
	return ok
}

func (ui *UI) moveActionFocus(key tcell.Key) {
	if ui.currentActions == nil {
		return
	}
	_, button := ui.currentActions.GetFocusedItemIndex()
	count := ui.currentActions.GetButtonCount()
	if count == 0 {
		return
	}
	if button < 0 {
		button = 0
	}
	switch key {
	case tcell.KeyUp, tcell.KeyLeft:
		button--
		if button < 0 {
			button = count - 1
		}
	case tcell.KeyDown, tcell.KeyRight:
		button++
		if button >= count {
			button = 0
		}
	}
	ui.tapp.SetFocus(ui.currentActions.GetButton(button))
}

func (ui *UI) switchSection(index int) {
	if index < 0 || index >= len(sectionPages) {
		return
	}
	ui.navigation.SetCurrentItem(index)
	ui.show(sectionPages[index])
	ui.tapp.SetFocus(ui.navigation)
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
	ui.currentActions = nil
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
	ui.currentContent = primitive
	ui.pages.AddAndSwitchToPage(page, primitive, true)
	ui.tapp.SetFocus(primitive)
	ui.header.SetText(ui.headerText())
	ui.footer.SetText(helpFor(page))
}

func (ui *UI) showDynamic(page string, primitive tview.Primitive) {
	ui.history = pushNavigation(ui.history, ui.current, page)
	ui.dynamicPages[page] = primitive
	ui.current = page
	ui.currentContent = primitive
	ui.currentActions = nil
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
	return "Tab next layer • Shift+Tab previous • Esc back • Arrows move • Enter select • q quit"
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
