package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (ui *UI) registerActions(actions *tview.Form) {
	ui.currentActions = actions
}

func (ui *UI) registerContent(content tview.Primitive) {
	ui.contentFocus = content
}

func newTable(headers []string, rows [][]string) *tview.Table {
	table := tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 1)
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetSelectable(false).
			SetTextColor(tview.Styles.PrimaryTextColor).
			SetAttributes(tcell.AttrBold))
	}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			table.SetCell(rowIndex+1, colIndex, tview.NewTableCell(value))
		}
	}
	return table
}

func (ui *UI) dashboardPage() tview.Primitive {
	runs, err := ui.app.ListAndReconcile(ui.ctx)
	if err != nil {
		return ui.errorPage(err)
	}
	imageRows, imageErr := ui.app.ImageList(ui.ctx)
	if imageErr != nil {
		return ui.errorPage(imageErr)
	}
	images, err := newImageRows(imageRows)
	if err != nil {
		return ui.errorPage(err)
	}
	gatewayRows, gatewayErr := ui.app.GatewayList(ui.ctx)
	if gatewayErr != nil {
		return ui.errorPage(gatewayErr)
	}
	gateways, err := newGatewayRows(gatewayRows)
	if err != nil {
		return ui.errorPage(err)
	}
	profiles, err := ui.app.Store.ListProfiles()
	if err != nil {
		return ui.errorPage(err)
	}
	projects, err := ui.app.Store.ListProjects()
	if err != nil {
		return ui.errorPage(err)
	}
	summary := newSummary(runs, len(images), len(gateways), len(profiles), len(projects))
	table := newTable(
		[]string{"Metric", "Value"},
		[][]string{
			{"Active runs", itoa(summary.Active)},
			{"Finished runs", itoa(summary.Finished)},
			{"Failed runs", itoa(summary.Failed)},
			{"Unsynced runs", itoa(summary.Unsynced)},
			{"Images", itoa(summary.Images)},
			{"Gateways", itoa(summary.Gateways)},
			{"Profiles", itoa(summary.Profiles)},
			{"Projects", itoa(summary.Projects)},
		},
	)
	table.SetBorder(true).SetTitle(" Dashboard ")
	ui.registerContent(table)
	quickActions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Start run", func() { ui.show("run-form") }).
		AddButton("Build image", func() { ui.show("image-build-form") }).
		AddButton("Connect gateway", func() { ui.show("gateway-form") })
	quickActions.SetBorder(true).SetTitle(" Quick actions ")
	ui.registerActions(quickActions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 3, true).
		AddItem(quickActions, 5, 0, false)
}

func (ui *UI) runsPage() tview.Primitive {
	runs, err := ui.app.ListAndReconcile(ui.ctx)
	if err != nil {
		return ui.errorPage(err)
	}
	rows := newRunRows(runs)
	selected := ""
	if len(rows) > 0 {
		selected = rows[0].ID
	}
	table := newTable(
		[]string{"Name", "Project", "Agent", "Mode", "Approval", "Isolation", "Status", "Synced", "Worktree"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Runs ")
	ui.registerContent(table)
	table.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(rows) {
			selected = rows[row-1].ID
		}
	})
	if len(rows) > 0 {
		table.Select(1, 0)
	}
	actions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Start", func() { ui.show("run-form") }).
		AddButton("Attach", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.suspend(func() error { return ui.app.Attach(ui.ctx, selected) })
		}).
		AddButton("Logs", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.showLogs(selected)
		}).
		AddButton("Sync", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.async(func() error { return ui.app.Sync(ui.ctx, selected) })
		}).
		AddButton("Stop", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.async(func() error { return ui.app.Stop(ui.ctx, selected) })
		}).
		AddButton("Diff", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.showDiff(selected)
		}).
		AddButton("Delete", func() {
			if !ui.requireRunSelection(selected) {
				return
			}
			ui.confirmDelete(selected)
		})
	actions.SetBorder(true).SetTitle(" Run actions ")
	ui.registerActions(actions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(actions, 6, 0, false)
}

func (ui *UI) imagesPage() tview.Primitive {
	data, err := ui.app.ImageList(ui.ctx)
	if err != nil {
		return ui.errorPage(err)
	}
	rows, err := newImageRows(data)
	if err != nil {
		return ui.errorPage(err)
	}
	table := newTable(
		[]string{"ID", "Name", "Agent", "Version", "Recipe", "Base", "Size", "Created"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Images ")
	ui.registerContent(table)
	actions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Build", func() { ui.show("image-build-form") }).
		AddButton("Push", func() { ui.show("image-push-form") }).
		AddButton("Pull", func() { ui.show("image-pull-form") }).
		AddButton("Registry", func() { ui.show(pageImageRegistry) })
	actions.SetBorder(true).SetTitle(" Image actions ")
	ui.registerActions(actions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(actions, 5, 0, false)
}

func (ui *UI) imageRegistryPage() tview.Primitive {
	registry, err := ui.app.Registry()
	if err != nil {
		return ui.errorPage(err)
	}
	reference := registry.NormalizedReference()
	rows := make([][]string, 0, len(ui.registryImages))
	for _, image := range ui.registryImages {
		rows = append(rows, []string{image.Agent, image.Version, itoa(image.Recipe), image.OS, image.Arch, image.Reference})
	}
	table := newTable([]string{"Agent", "Version", "Recipe", "OS", "Arch", "Reference"}, rows)
	table.SetBorder(true).SetTitle(" External registry images ")
	ui.registerContent(table)

	referenceField := tview.NewInputField().
		SetLabel("Registry repository").
		SetText(reference).
		SetFieldWidth(80)
	form := tview.NewForm().
		AddFormItem(referenceField).
		AddButton("Save", func() {
			value := strings.TrimSpace(referenceField.GetText())
			ui.asyncTo(pageImageRegistry, func() error { return ui.app.SetRegistry(value) })
		}).
		AddButton("Discover", func() {
			value := strings.TrimSpace(referenceField.GetText())
			ui.asyncTo(pageImageRegistry, func() error {
				if err := ui.app.SetRegistry(value); err != nil {
					return err
				}
				images, err := ui.app.RegistryDiscover(ui.ctx)
				if err != nil {
					return err
				}
				ui.registryImages = images
				return nil
			})
		}).
		AddButton("Clear", func() {
			ui.asyncTo(pageImageRegistry, func() error {
				ui.registryImages = nil
				return ui.app.ClearRegistry()
			})
		}).
		AddButton("Cancel", func() { ui.show(pageImages) })
	form.SetBorder(true).SetTitle(" Registry actions ")
	ui.registerActions(form)

	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(form, 8, 0, false)
}

func (ui *UI) gatewaysPage() tview.Primitive {
	data, err := ui.app.GatewayList(ui.ctx)
	if err != nil {
		return ui.errorPage(err)
	}
	rows, err := newGatewayRows(data)
	if err != nil {
		return ui.errorPage(err)
	}
	table := newTable(
		[]string{"Name", "Active", "Endpoint", "Type", "Auth", "Source"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Gateways ")
	ui.registerContent(table)
	actions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Connect", func() { ui.show("gateway-form") }).
		AddButton("Start", func() {
			ui.runTask("Start gateway", pageGateways, true, func(ctx context.Context, a *app.App, _ io.Writer) error {
				return a.GatewayStart(ctx, false)
			})
		}).
		AddButton("Inspect", func() { ui.show("gateway-info-form") })
	actions.SetBorder(true).SetTitle(" Gateway actions ")
	ui.registerActions(actions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(actions, 5, 0, false)
}

func (ui *UI) policiesPage() tview.Primitive {
	runs, err := ui.app.Store.ListRuns()
	if err != nil {
		return ui.errorPage(err)
	}
	if len(runs) == 0 {
		return ui.emptyPage("No runs yet. Start a run to inspect policies.")
	}
	policies, err := ui.app.PolicyShow(ui.ctx, runs[0].ID)
	if err != nil {
		return ui.errorPage(err)
	}
	left := tview.NewFlex().SetDirection(tview.FlexRow)
	ui.registerContent(left)
	for _, key := range sortedJSONKeys(policies) {
		tree, err := jsonTree(policies[key], strings.Title(key))
		if err != nil {
			return ui.errorPage(err)
		}
		tree.SetBorder(true).SetTitle(" " + strings.Title(key) + " ")
		left.AddItem(tree, 0, 1, true)
	}
	form := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Allow host", func() { ui.show("policy-host-form") }).
		AddButton("Edit YAML", func() { ui.suspend(func() error { return ui.app.PolicyEdit(ui.ctx, runs[0].ID, "", "", false) }) })
	form.SetBorder(true).SetTitle(" Policy actions ")
	return tview.NewFlex().
		AddItem(left, 0, 4, true).
		AddItem(form, 24, 0, false)
}

func (ui *UI) showProfileEdit(name string) {
	profile, err := ui.app.Store.LoadProfile(name)
	if err != nil {
		ui.message("[red]Error: %s", err)
		return
	}
	ui.showDynamic("profile-edit:"+name, ui.profileEditForm(profile))
}

func (ui *UI) showProjectEdit(name string) {
	project, err := ui.app.Store.LoadProject(name)
	if err != nil {
		ui.message("[red]Error: %s", err)
		return
	}
	ui.showDynamic("project-edit:"+name, ui.projectEditForm(project))
}

func (ui *UI) settingsPage() tview.Primitive {
	settings, err := ui.app.EffectiveSettings()
	if err != nil {
		return ui.errorPage(err)
	}
	root, prefix := settings.Worktrees.Root, settings.Worktrees.BranchPrefix
	form := tview.NewForm().
		AddInputField("Worktree root", root, 90, nil, func(text string) { root = text }).
		AddInputField("Branch prefix", prefix, 30, nil, func(text string) { prefix = text }).
		AddTextView("Worktrees", "New isolated runs use <root>/<project>/<run-name> and <prefix>/<project>/<run-name>. Existing runs keep their recorded paths and branches.", 100, 3, false, false).
		AddButton("Save", func() {
			ui.asyncTo(pageSettings, func() error {
				_, err := ui.app.SaveGlobalSettings(root, prefix)
				return err
			})
		})
	form.SetBorder(true).SetTitle(" Settings ")
	return form
}

func (ui *UI) projectsPage() tview.Primitive {
	projects, err := ui.app.Store.ListProjects()
	if err != nil {
		return ui.errorPage(err)
	}
	rows := newProjectRows(projects)
	selected := ""
	if len(rows) > 0 {
		selected = rows[0].Name
	}
	table := newTable(
		[]string{"Name", "Root", "Isolation"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Projects ")
	ui.registerContent(table)
	table.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(rows) {
			selected = rows[row-1].Name
		}
	})
	if len(rows) > 0 {
		table.Select(1, 0)
	}
	actions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Create", func() { ui.show("project-form") }).
		AddButton("Edit", func() {
			if selected != "" {
				ui.showProjectEdit(selected)
			}
		})
	actions.SetBorder(true).SetTitle(" Project actions ")
	ui.registerActions(actions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(actions, 5, 0, false)
}

func (ui *UI) profilesPage() tview.Primitive {
	profiles, err := ui.app.Store.ListProfiles()
	if err != nil {
		return ui.errorPage(err)
	}
	rows := newProfileRows(profiles)
	selected := ""
	if len(rows) > 0 {
		selected = rows[0].Name
	}
	table := newTable(
		[]string{"Name", "Agent", "Mode", "Model", "Provider", "Approval"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Profiles ")
	ui.registerContent(table)
	table.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(rows) {
			selected = rows[row-1].Name
		}
	})
	if len(rows) > 0 {
		table.Select(1, 0)
	}
	actions := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Create", func() { ui.show("profile-form") }).
		AddButton("Edit", func() {
			if selected != "" {
				ui.showProfileEdit(selected)
			}
		}).
		AddButton("Provider setup", func() { ui.show("provider-form") })
	actions.SetBorder(true).SetTitle(" Profile actions ")
	ui.registerActions(actions)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 4, true).
		AddItem(actions, 5, 0, false)
}

func (ui *UI) doctorPage() tview.Primitive {
	checks := ui.app.Doctor(ui.ctx)
	rows := newDoctorRows(checks)
	table := newTable(
		[]string{"Check", "Status", "Detail"},
		toMatrix(rows),
	)
	table.SetBorder(true).SetTitle(" Doctor ")
	ui.registerContent(table)
	form := tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		AddButton("Refresh", func() { ui.refresh() })
	form.SetBorder(true).SetTitle(" Actions ")
	ui.registerActions(form)
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 5, true).
		AddItem(form, 5, 0, false)
}

func (ui *UI) showLogs(id string) {
	logs, err := ui.app.Logs(ui.ctx, id)
	if err != nil {
		ui.message("[red]Error: %s", err)
		return
	}
	view := tview.NewTextView().
		SetText(string(logs)).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(false)
	view.SetBorder(true).SetTitle(" Logs: " + id + " ")
	ui.showDynamic("logs:"+id, view)
}

func (ui *UI) showDiff(id string) {
	run, err := ui.app.Store.LoadRun(id)
	if err != nil {
		ui.message("[red]Error: %s", err)
		return
	}
	diff, err := ui.app.Diff(ui.ctx, run)
	if err != nil {
		ui.message("[red]Error: %s", err)
		return
	}
	view := tview.NewTextView().
		SetText(string(diff)).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(false)
	view.SetBorder(true).SetTitle(" Diff: " + id + " ")
	ui.showDynamic("diff:"+id, view)
}

func (ui *UI) confirmDelete(id string) {
	ui.modalActive = true
	modal := tview.NewModal().
		SetText("Delete this run and discard its results?").
		AddButtons([]string{"Cancel", "Delete"}).
		SetDoneFunc(func(index int, label string) {
			ui.modalActive = false
			if index == 1 {
				ui.async(func() error { return ui.app.Delete(ui.ctx, id, true) })
			}
			ui.pages.RemovePage("modal")
			ui.show(pageRuns)
		})
	ui.pages.AddAndSwitchToPage("modal", modal, true)
}

func (ui *UI) errorPage(err error) tview.Primitive {
	view := tview.NewTextView().
		SetText("Error: " + err.Error()).
		SetTextColor(tview.Styles.PrimaryTextColor)
	view.SetBorder(true).SetTitle(" Error ")
	return view
}

func (ui *UI) emptyPage(message string) tview.Primitive {
	view := tview.NewTextView().SetText(message)
	view.SetBorder(true).SetTitle(" Empty ")
	ui.registerContent(view)
	return view
}

func toMatrix[T any](rows []T) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToStrings(row))
	}
	return out
}

func rowToStrings[T any](row T) []string {
	switch typed := any(row).(type) {
	case runRow:
		return []string{typed.Name, typed.Project, typed.Agent, typed.Mode, typed.Approval, typed.Isolation, typed.Status, typed.Synced, typed.Worktree}
	case imageRow:
		return []string{typed.ID, typed.Name, typed.Agent, typed.Version, typed.Recipe, typed.BaseDigest, typed.Size, typed.Created}
	case gatewayRow:
		return []string{typed.Name, typed.Active, typed.Endpoint, typed.Type, typed.Auth, typed.Source}
	case projectRow:
		return []string{typed.Name, typed.Root, typed.Isolation}
	case profileRow:
		return []string{typed.Name, typed.Agent, typed.Mode, typed.Model, typed.Provider, typed.Approval}
	case doctorRow:
		return []string{typed.Name, typed.Status, typed.Detail}
	default:
		return []string{fmt.Sprintf("%v", typed)}
	}
}
