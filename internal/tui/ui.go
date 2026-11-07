package tui

import (
	"context"
	"fmt"

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
	ctx        context.Context
	app        *app.App
	tapp       *tview.Application
	pages      *tview.Pages
	navigation *tview.List
	header     *tview.TextView
	footer     *tview.TextView
	current    string
}

// Run starts the AgentBox terminal user interface.
func Run(ctx context.Context, a *app.App) error {
	if a == nil {
		return fmt.Errorf("application is required")
	}
	ui := &UI{
		ctx:  ctx,
		app:  a,
		tapp: tview.NewApplication(),
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
		ShowSecondaryText(false).
		AddItem("Dashboard", "", 'd', func() { ui.show(pageDashboard) }).
		AddItem("Runs", "", 'r', func() { ui.show(pageRuns) }).
		AddItem("Images", "", 'i', func() { ui.show(pageImages) }).
		AddItem("Gateways", "", 'g', func() { ui.show(pageGateways) }).
		AddItem("Policies", "", 'p', func() { ui.show(pagePolicies) }).
		AddItem("Profiles", "", 'o', func() { ui.show(pageProfiles) }).
		AddItem("Doctor", "", 'x', func() { ui.show(pageDoctor) })
	ui.navigation.SetBorder(true).SetTitle(" Navigation ")

	ui.footer = tview.NewTextView().
		SetDynamicColors(true).
		SetText("F1–F7 switch sections • Ctrl+←/→ cycles • Enter selects • q quits")
	ui.footer.SetBorder(true).SetTitle(" Help ")

	ui.pages = tview.NewPages()
	ui.show(pageDashboard)
}

func (ui *UI) handleGlobalKeys(event *tcell.EventKey) *tcell.EventKey {
	if index, ok := sectionForFunctionKey(event.Key()); ok {
		ui.switchSection(index)
		return nil
	}
	switch event.Key() {
	case tcell.KeyLeft:
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			ui.show(nextSection(ui.current, -1))
			return nil
		}
	case tcell.KeyRight:
		if event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			ui.show(nextSection(ui.current, 1))
			return nil
		}
	}
	return event
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

func (ui *UI) switchSection(index int) {
	if index < 0 || index >= len(sectionPages) {
		return
	}
	ui.navigation.SetCurrentItem(index)
	ui.show(sectionPages[index])
}

func (ui *UI) root() tview.Primitive {
	body := tview.NewFlex().
		AddItem(ui.navigation, 22, 1, false).
		AddItem(ui.pages, 0, 8, true)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(ui.header, 3, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(ui.footer, 3, 0, false)
}

func (ui *UI) show(page string) {
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
		primitive = tview.NewTextView().SetText("Unknown page")
	}
	ui.pages.AddAndSwitchToPage(page, primitive, true)
	ui.header.SetText("[::b]AgentBox — " + pageTitle(page) + "[::-]")
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
		return "Unknown"
	}
}

func (ui *UI) message(format string, args ...any) {
	ui.footer.SetText(fmt.Sprintf(format, args...))
}

func (ui *UI) refresh() {
	ui.show(ui.current)
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
