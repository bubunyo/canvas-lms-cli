package canvas

import (
	"bytes"
	"encoding/json"
	"time"
)

// ID accepts both numeric and string JSON ids (Canvas returns either,
// depending on the application/json+canvas-string-ids Accept header).
type ID string

func (id *ID) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	*id = ID(bytes.Trim(b, `"`))
	return nil
}

type User struct {
	ID        ID     `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	LoginID   string `json:"login_id"`
	Email     string `json:"primary_email"`
}

type AccessToken struct {
	ID           ID         `json:"id"`
	Purpose      string     `json:"purpose"`
	Token        string     `json:"token"`
	VisibleToken string     `json:"visible_token"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

type Term struct {
	Name string `json:"name"`
}

type Course struct {
	ID            ID         `json:"id"`
	Name          string     `json:"name"`
	CourseCode    string     `json:"course_code"`
	WorkflowState string     `json:"workflow_state"`
	StartAt       *time.Time `json:"start_at"`
	EndAt         *time.Time `json:"end_at"`
	Term          *Term      `json:"term"`
}

type CompletionRequirement struct {
	Type      string `json:"type"`
	Completed bool   `json:"completed"`
}

type ModuleItem struct {
	ID                    ID                     `json:"id"`
	Title                 string                 `json:"title"`
	Type                  string                 `json:"type"`
	Indent                int                    `json:"indent"`
	HTMLURL               string                 `json:"html_url"`
	PageURL               string                 `json:"page_url"`
	ContentID             ID                     `json:"content_id"`
	ExternalURL           string                 `json:"external_url"`
	CompletionRequirement *CompletionRequirement `json:"completion_requirement"`
}

type Module struct {
	ID         ID           `json:"id"`
	Name       string       `json:"name"`
	Position   int          `json:"position"`
	State      string       `json:"state"`
	ItemsCount int          `json:"items_count"`
	Items      []ModuleItem `json:"items"`
}

type PlannerSubmission struct {
	Submitted bool `json:"submitted"`
	Excused   bool `json:"excused"`
	Graded    bool `json:"graded"`
	Late      bool `json:"late"`
	Missing   bool `json:"missing"`
}

type PlannerItem struct {
	CourseID       ID              `json:"course_id"`
	ContextName    string          `json:"context_name"`
	PlannableType  string          `json:"plannable_type"`
	PlannableDate  time.Time       `json:"plannable_date"`
	HTMLURL        string          `json:"html_url"`
	NewActivity    bool            `json:"new_activity"`
	RawSubmissions json.RawMessage `json:"submissions"` // false, or a PlannerSubmission object
	Plannable      struct {
		Title          string     `json:"title"`
		DueAt          *time.Time `json:"due_at"`
		PointsPossible *float64   `json:"points_possible"`
	} `json:"plannable"`
	PlannerOverride *struct {
		MarkedComplete bool `json:"marked_complete"`
	} `json:"planner_override"`
}

func (p PlannerItem) Submission() *PlannerSubmission {
	var s PlannerSubmission
	if len(p.RawSubmissions) == 0 || p.RawSubmissions[0] != '{' || json.Unmarshal(p.RawSubmissions, &s) != nil {
		return nil
	}
	return &s
}

type Page struct {
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	HTMLURL   string     `json:"html_url"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type DiscussionTopic struct {
	Title   string     `json:"title"`
	Message string     `json:"message"`
	HTMLURL string     `json:"html_url"`
	DueAt   *time.Time `json:"todo_date"`
}

type Quiz struct {
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	HTMLURL        string     `json:"html_url"`
	DueAt          *time.Time `json:"due_at"`
	PointsPossible *float64   `json:"points_possible"`
}

type File struct {
	ID          ID     `json:"id"`
	DisplayName string `json:"display_name"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content-type"`
}

type Assignment struct {
	ID             ID         `json:"id"`
	Name           string     `json:"name"`
	DueAt          *time.Time `json:"due_at"`
	PointsPossible *float64   `json:"points_possible"`
	HTMLURL        string     `json:"html_url"`
	Description    string     `json:"description"`
	CourseID       ID         `json:"course_id"`
	Course         *Course    `json:"course"`
}
