// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package create

import (
	"errors"
	"fmt"
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	util "github.com/daytonaio/daytona/internal/util"
	"github.com/daytonaio/daytona/pkg/apiclient"
	"github.com/daytonaio/daytona/pkg/views"
	views_util "github.com/daytonaio/daytona/pkg/views/util"
)

type ProjectDetail string

const (
	Build              ProjectDetail = "Build"
	DevcontainerConfig ProjectDetail = "Devcontainer Config"
	Image              ProjectDetail = "Image"
	User               ProjectDetail = "User"
	EnvVars            ProjectDetail = "Env Vars"
	EMPTY_STRING                     = ""
	DEFAULT_PADDING                  = 21
)

type SummaryModel struct {
	lg          *lipgloss.Renderer
	styles      *Styles
	form        *huh.Form
	width       int
	quitting    bool
	name        string
	projectList []apiclient.CreateProjectDTO
	defaults    *views_util.ProjectConfigDefaults
	nameLabel   string
}

type SubmissionFormConfig struct {
	ChosenName    *string
	SuggestedName string
	ExistingNames []string
	ProjectList   *[]apiclient.CreateProjectDTO
	NameLabel     string
	Defaults      *views_util.ProjectConfigDefaults
}

var configureCheck bool
var userCancelled bool
var ProjectsConfigurationChanged bool

func RunSubmissionForm(config SubmissionFormConfig, pcImport *bool) error {
	configureCheck = false

	m := NewSummaryModel(config, pcImport)

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return err
	}

	if userCancelled {
		return errors.New("user cancelled")
	}

	if !configureCheck {
		return nil
	}

	if config.Defaults.Image == nil || config.Defaults.ImageUser == nil {
		return errors.New("default project entries are not set")
	}

	var err error
	ProjectsConfigurationChanged, err = RunProjectConfiguration(config.ProjectList, *config.Defaults, *pcImport)
	if err != nil {
		return err
	}

	return RunSubmissionForm(config, pcImport)
}

func RenderSummary(name string, projectList []apiclient.CreateProjectDTO, defaults *views_util.ProjectConfigDefaults, nameLabel string) (string, error) {
	var output string
	if name == "" {
		output = views.GetStyledMainTitle("SUMMARY")
	} else {
		output = views.GetStyledMainTitle(fmt.Sprintf("SUMMARY - %s %s", nameLabel, name))
	}

	output += "\n\n"

	for i := range projectList {
		if len(projectList) == 1 {
			output += fmt.Sprintf("%s - %s\n", lipgloss.NewStyle().Foreground(views.Green).Render("Project"), (projectList[i].Source.Repository.Url))
		} else {
			output += fmt.Sprintf("%s - %s\n", lipgloss.NewStyle().Foreground(views.Green).Render(fmt.Sprintf("%s #%d", "Project", i+1)), (projectList[i].Source.Repository.Url))
		}

		projectBuildChoice, choiceName := views_util.GetProjectBuildChoice(projectList[i], defaults)
		output += renderProjectDetails(projectList[i], projectBuildChoice, choiceName)
		if i < len(projectList)-1 {
			output += "\n\n"
		}
	}

	return output, nil
}

func renderProjectDetails(project apiclient.CreateProjectDTO, buildChoice views_util.BuildChoice, choiceName string) string {
	output := projectDetailOutput(Build, choiceName)

	if buildChoice == views_util.DEVCONTAINER {
		if project.BuildConfig != nil {
			if project.BuildConfig.Devcontainer != nil {
				output += "\n"
				output += projectDetailOutput(DevcontainerConfig, project.BuildConfig.Devcontainer.FilePath)
			}
		}
	} else {
		if project.Image != nil {
			if output != "" {
				output += "\n"
			}
			output += projectDetailOutput(Image, *project.Image)
		}

		if project.User != nil {
			if output != "" {
				output += "\n"
			}
			output += projectDetailOutput(User, *project.User)
		}
	}

	if len(project.EnvVars) > 0 {
		if output != "" {
			output += "\n"
		}

		var envVars string
		for key, val := range project.EnvVars {
			envVars += fmt.Sprintf("%s=%s; ", key, val)
		}
		output += projectDetailOutput(EnvVars, strings.TrimSuffix(envVars, "; "))
	}

	return output
}

func projectDetailOutput(projectDetailKey ProjectDetail, projectDetailValue string) string {
	return fmt.Sprintf("\t%s%-*s%s", lipgloss.NewStyle().Foreground(views.Green).Render(string(projectDetailKey)), DEFAULT_PADDING-len(string(projectDetailKey)), EMPTY_STRING, projectDetailValue)
}

func NewSummaryModel(config SubmissionFormConfig, pcImport *bool) SummaryModel {
	m := SummaryModel{width: maxWidth}
	m.lg = lipgloss.DefaultRenderer()
	m.styles = NewStyles(m.lg)
	m.name = *config.ChosenName
	m.projectList = *config.ProjectList
	m.defaults = config.Defaults
	m.nameLabel = config.NameLabel

	if *config.ChosenName == "" {
		*config.ChosenName = config.SuggestedName
	}

	if !*pcImport {
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("%s name", config.NameLabel)).
					Value(config.ChosenName).
					Key("name").
					Validate(func(str string) error {
						result, err := util.GetValidatedName(str)
						if err != nil {
							return err
						}
						for _, name := range config.ExistingNames {
							if name == result {
								return errors.New("name already exists")
							}
						}
						*config.ChosenName = result
						return nil
					}),
			),
		).WithShowHelp(false).WithTheme(views.GetCustomTheme())
	} else {
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Is the above information correct?").
					Value(pcImport),
			),
		).WithShowHelp(false).WithTheme(views.GetCustomTheme())
	}

	return m
}

func (m SummaryModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m SummaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			userCancelled = true
			m.quitting = true
			return m, tea.Quit
		case "f10":
			m.quitting = true
			m.form.State = huh.StateCompleted
			configureCheck = true
			return m, tea.Quit
		}
	}

	var cmds []tea.Cmd

	// Process the form
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
		cmds = append(cmds, cmd)
	}

	if m.form.State == huh.StateCompleted {
		// Quit when the form is done.
		m.quitting = true
		cmds = append(cmds, tea.Quit)
	}

	return m, tea.Batch(cmds...)
}

func (m SummaryModel) View() string {
	if m.quitting {
		return ""
	}

	view := m.form.WithHeight(5).View() + "\n" + configurationHelpLine

	if len(m.projectList) > 1 || len(m.projectList) == 1 && ProjectsConfigurationChanged {
		summary, err := RenderSummary(m.name, m.projectList, m.defaults, m.nameLabel)
		if err != nil {
			log.Fatal(err)
		}
		view = views.GetBorderedMessage(summary) + "\n" + view
	}

	return view
}
