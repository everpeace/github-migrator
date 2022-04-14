package migrator

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/itchyny/github-migrator/github"
	"github.com/itchyny/github-migrator/repo"
)

func init() {
	beforeImportIssueDuration = 0
	waitImportIssueInitialDuration = 0
	waitProjectColumnDuration = 0
	waitProjectCardDuration = 0
}

type testRepo struct {
	Repo         *github.Repo
	UpdateRepo   *github.Repo            `json:"update_repo"`
	Members      []*github.Member        `json:"members"`
	UserByNames  map[string]*github.User `json:"users"`
	Labels       []*github.Label         `json:"labels"`
	CreateLabels []*github.Label         `json:"create_labels"`
	UpdateLabels []*github.Label         `json:"update_labels"`
	Issues       []struct {
		*github.PullReq
		Comments       []*github.Comment       `json:"comments"`
		Events         []*github.Event         `json:"events"`
		Commits        []*github.Commit        `json:"commit_details"`
		Reviews        []*github.Review        `json:"reviews"`
		ReviewComments []*github.ReviewComment `json:"review_comments"`
	}
	Compare  map[string]string
	Imports  []*github.Import `json:"imports"`
	Projects []*struct {
		*github.Project
		Columns []*testProjectColumn `json:"columns"`
	} `json:"projects"`
	CreateProjects       []*github.Project                 `json:"create_projects"`
	UpdateProjects       []*github.Project                 `json:"update_projects"`
	CreateProjectColumns []*github.ProjectColumn           `json:"create_project_columns"`
	CreateProjectCards   []*github.CreateProjectCardParams `json:"create_project_cards"`
	Milestones           []*github.Milestone               `json:"milestones"`
	CreateMilestones     []*github.Milestone               `json:"create_milestones"`
	UpdateMilestones     []*github.Milestone               `json:"update_milestones"`
	Hooks                []*github.Hook                    `json:"hooks"`
	CreateHooks          []*github.Hook                    `json:"create_hooks"`
	UpdateHooks          []*github.Hook                    `json:"update_hooks"`
}

type testProjectColumn struct {
	*github.ProjectColumn
	Cards []*github.ProjectCard `json:"cards"`
}

func (r *testRepo) build(t *testing.T, isTarget bool) *repo.Repo {
	projects := make([]*github.Project, len(r.Projects))
	for i, p := range r.Projects {
		projects[i] = p.Project
	}
	milestones := make([]*github.Milestone, len(r.Milestones))
	copy(milestones, r.Milestones)

	return repo.New(github.NewMockClient(

		github.MockGetUser(func(name string) (*github.User, error) {
			assert.True(t, isTarget)
			if u, ok := r.UserByNames[name]; ok {
				return u, nil
			}
			return nil, fmt.Errorf("user not found: %s", name)
		}),
		github.MockListMembers(func(string) github.Members {
			assert.True(t, isTarget)
			return github.MembersFromSlice(r.Members)
		}),

		github.MockGetRepo(func(string) (*github.Repo, error) {
			return r.Repo, nil
		}),
		github.MockUpdateRepo(func(_ string, params *github.UpdateRepoParams) (*github.Repo, error) {
			assert.True(t, isTarget)
			assert.NotNil(t, r.UpdateRepo)
			assert.Equal(t, r.UpdateRepo.Name, params.Name)
			assert.Equal(t, r.UpdateRepo.Description, params.Description)
			assert.Equal(t, r.UpdateRepo.Homepage, params.Homepage)
			assert.Equal(t, r.UpdateRepo.Private, params.Private)
			return r.UpdateRepo, nil
		}),

		github.MockListLabels(func(string) github.Labels {
			return github.LabelsFromSlice(r.Labels)
		}),
		github.MockCreateLabel((func(i int) func(string, *github.CreateLabelParams) (*github.Label, error) {
			return func(_ string, params *github.CreateLabelParams) (*github.Label, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateLabels), i)
				assert.Equal(t, r.CreateLabels[i].Name, params.Name)
				assert.Equal(t, r.CreateLabels[i].Color, params.Color)
				assert.Equal(t, r.CreateLabels[i].Description, params.Description)
				return nil, nil
			}
		})(0)),
		github.MockUpdateLabel((func(i int) func(string, string, *github.UpdateLabelParams) (*github.Label, error) {
			return func(path, name string, params *github.UpdateLabelParams) (*github.Label, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateLabels), i)
				assert.Equal(t, r.UpdateLabels[i].Name, name)
				assert.Equal(t, r.UpdateLabels[i].Name, params.Name)
				assert.Equal(t, r.UpdateLabels[i].Color, params.Color)
				assert.Equal(t, r.UpdateLabels[i].Description, params.Description)
				return nil, nil
			}
		})(0)),

		github.MockListIssues(func(string, *github.ListIssuesParams) github.Issues {
			xs := make([]*github.Issue, len(r.Issues))
			for i, s := range r.Issues {
				xs[i] = &s.PullReq.Issue
			}
			return github.IssuesFromSlice(xs)
		}),
		github.MockGetIssue(func(_ string, issueNumber int) (*github.Issue, error) {
			for _, i := range r.Issues {
				if i.Number == issueNumber {
					return &i.PullReq.Issue, nil
				}
			}
			panic(fmt.Sprintf("unexpected issue number: %d", issueNumber))
		}),
		github.MockListComments(func(_ string, issueNumber int) github.Comments {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.Issue.Number == issueNumber {
					return github.CommentsFromSlice(s.Comments)
				}
			}
			panic(fmt.Sprintf("unexpected issue number: %d", issueNumber))
		}),
		github.MockListEvents(func(_ string, issueNumber int) github.Events {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.Issue.Number == issueNumber {
					return github.EventsFromSlice(s.Events)
				}
			}
			panic(fmt.Sprintf("unexpected issue number: %d", issueNumber))
		}),

		github.MockGetPullReq(func(_ string, pullNumber int) (*github.PullReq, error) {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return s.PullReq, nil
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockListPullReqCommits(func(_ string, pullNumber int) github.Commits {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.CommitsFromSlice(s.Commits)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockGetCompare(func(_ string, base, head string) (string, error) {
			assert.True(t, !isTarget)
			if diff, ok := r.Compare[base+"..."+head]; ok {
				return diff, nil
			}
			panic(fmt.Sprintf("unexpected compare: %s...%s", base, head))
		}),
		github.MockListReviews(func(_ string, pullNumber int) github.Reviews {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.ReviewsFromSlice(s.Reviews)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockListReviewComments(func(_ string, pullNumber int) github.ReviewComments {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.ReviewCommentsFromSlice(s.ReviewComments)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),

		github.MockListProjects(func(string, *github.ListProjectsParams) github.Projects {
			return github.ProjectsFromSlice(projects)
		}),
		github.MockGetProject(func(projectID int) (*github.Project, error) {
			assert.True(t, !isTarget)
			for _, p := range r.Projects {
				if p.ID == projectID {
					return p.Project, nil
				}
			}
			panic(fmt.Sprintf("unexpected project id: %d", projectID))
		}),
		github.MockCreateProject((func(i int) func(string, *github.CreateProjectParams) (*github.Project, error) {
			return func(_ string, params *github.CreateProjectParams) (*github.Project, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateProjects), i)
				assert.Equal(t, r.CreateProjects[i].Name, params.Name)
				assert.Equal(t, r.CreateProjects[i].Body, params.Body)
				projects = append(projects, r.CreateProjects[i])
				return r.CreateProjects[i], nil
			}
		})(0)),
		github.MockUpdateProject((func(i int) func(int, *github.UpdateProjectParams) (*github.Project, error) {
			return func(projectID int, params *github.UpdateProjectParams) (*github.Project, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateProjects), i)
				assert.Equal(t, "", params.Name)
				assert.Equal(t, r.UpdateProjects[i].Body, params.Body)
				assert.Equal(t, r.UpdateProjects[i].State, params.State)
				return r.UpdateProjects[i], nil
			}
		})(0)),

		github.MockListProjectColumns(func(projectID int) github.ProjectColumns {
			for _, p := range r.Projects {
				if p.ID == projectID {
					cs := make([]*github.ProjectColumn, len(p.Columns))
					for i, c := range p.Columns {
						cs[i] = c.ProjectColumn
					}
					return github.ProjectColumnsFromSlice(cs)
				}
			}
			return github.ProjectColumnsFromSlice([]*github.ProjectColumn{})
		}),
		github.MockCreateProjectColumn((func(i int) func(int, string) (*github.ProjectColumn, error) {
			return func(projectID int, name string) (*github.ProjectColumn, error) {
				defer func() { i++ }()
				for _, p := range r.Projects {
					if p.ID == projectID {
						p.Columns = append(p.Columns, &testProjectColumn{
							ProjectColumn: &github.ProjectColumn{
								ID:   -1,
								Name: name,
							},
						})
					}
				}
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateProjectColumns), i)
				assert.Equal(t, r.CreateProjectColumns[i].Name, name)
				return r.CreateProjectColumns[i], nil
			}
		})(0)),

		github.MockListProjectCards(func(columnID int) github.ProjectCards {
			for _, p := range r.Projects {
				for _, c := range p.Columns {
					if c.ID == columnID {
						return github.ProjectCardsFromSlice(c.Cards)
					}
				}
			}
			return github.ProjectCardsFromSlice([]*github.ProjectCard{})
		}),
		github.MockCreateProjectCard((func(i int) func(int, *github.CreateProjectCardParams) (*github.ProjectCard, error) {
			return func(projectID int, params *github.CreateProjectCardParams) (*github.ProjectCard, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateProjectCards), i)
				assert.Equal(t, r.CreateProjectCards[i], params)
				return nil, nil
			}
		})(0)),

		github.MockListMilestones(func(string, *github.ListMilestonesParams) github.Milestones {
			return github.MilestonesFromSlice(milestones)
		}),
		github.MockCreateMilestone((func(i int) func(string, *github.CreateMilestoneParams) (*github.Milestone, error) {
			return func(_ string, params *github.CreateMilestoneParams) (*github.Milestone, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateMilestones), i)
				assert.Equal(t, r.CreateMilestones[i].Title, params.Title)
				assert.Equal(t, r.CreateMilestones[i].Description, params.Description)
				assert.Equal(t, r.CreateMilestones[i].State, params.State)
				milestones = append(milestones, r.CreateMilestones[i])
				return r.CreateMilestones[i], nil
			}
		})(0)),
		github.MockUpdateMilestone((func(i int) func(string, int, *github.UpdateMilestoneParams) (*github.Milestone, error) {
			return func(_ string, milestoneNumber int, params *github.UpdateMilestoneParams) (*github.Milestone, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateMilestones), i)
				assert.Equal(t, r.UpdateMilestones[i].Number, milestoneNumber)
				assert.Equal(t, r.UpdateMilestones[i].Title, params.Title)
				assert.Equal(t, r.UpdateMilestones[i].Description, params.Description)
				assert.Equal(t, r.UpdateMilestones[i].State, params.State)
				return r.UpdateMilestones[i], nil
			}
		})(0)),
		github.MockDeleteMilestone((func(i int) func(string, int) error {
			return func(string, int) error {
				return nil
			}
		})(0)),

		github.MockListHooks(func(string) github.Hooks {
			return github.HooksFromSlice(r.Hooks)
		}),
		github.MockCreateHook((func(i int) func(string, *github.CreateHookParams) (*github.Hook, error) {
			return func(_ string, params *github.CreateHookParams) (*github.Hook, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateHooks), i)
				assert.Equal(t, r.CreateHooks[i].Events, params.Events)
				assert.Equal(t, r.CreateHooks[i].Config, params.Config)
				assert.Equal(t, r.CreateHooks[i].Active, params.Active)
				return nil, nil
			}
		})(0)),
		github.MockUpdateHook((func(i int) func(string, int, *github.UpdateHookParams) (*github.Hook, error) {
			return func(_ string, hookID int, params *github.UpdateHookParams) (*github.Hook, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateHooks), i)
				assert.Equal(t, r.UpdateHooks[i].Events, params.Events)
				assert.Equal(t, r.UpdateHooks[i].Config, params.Config)
				assert.Equal(t, r.UpdateHooks[i].Active, params.Active)
				return nil, nil
			}
		})(0)),

		github.MockImport((func(i int) func(string, *github.Import) (*github.ImportResult, error) {
			return func(_ string, x *github.Import) (*github.ImportResult, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.Imports), i)
				assert.Equal(t, r.Imports[i], x)
				return &github.ImportResult{
					ID:     12345,
					Status: "pending",
					URL:    "http://localhost/repo/example/target/import/issues/12345",
				}, nil
			}
		})(0)),
		github.MockGetImport(func(_ string, id int) (*github.ImportResult, error) {
			assert.True(t, isTarget)
			assert.Equal(t, 12345, id)
			return &github.ImportResult{
				ID:     12345,
				Status: "imported",
				URL:    "http://localhost/repo/example/target/import/issues/12345",
			}, nil
		}),
	), r.Repo.FullName)
}

func TestMigratorMigrate(t *testing.T) {
	f, err := os.Open("test.yaml")
	require.NoError(t, err)
	defer f.Close()

	var testCases []struct {
		Name        string            `json:"name"`
		Source      *testRepo         `json:"source"`
		Target      *testRepo         `json:"target"`
		UserMapping map[string]string `json:"user_mapping"`
	}
	require.NoError(t, decodeYAML(f, &testCases))

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			source := tc.Source.build(t, false)
			target := tc.Target.build(t, true)
			migrator := New(source, target, tc.UserMapping)
			assert.Nil(t, migrator.Migrate())
		})
	}
}

func decodeYAML(r io.Reader, d interface{}) error {
	// decode to interface once to use json tags
	var m interface{}
	if err := yaml.NewDecoder(r).Decode(&m); err != nil {
		return err
	}
	bs, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, d)
}
