package activitypub

import (
	"time"

	"encoding/json"
)

type AtContextRaw struct {
	Context json.RawMessage `json:"@context,omitempty"`
}

type ActivityRaw struct {
	AtContextRaw
	Type      string          `json:"type,omitempty"`
	Id        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	ToRaw     json.RawMessage `json:"to,omitempty"`
	BtoRaw    json.RawMessage `json:"bto,omitempty"`
	CcRaw     json.RawMessage `json:"cc,omitempty"`
	Published time.Time       `json:"published,omitempty"`
	ActorRaw  json.RawMessage `json:"actor,omitempty"`
	ObjectRaw json.RawMessage `json:"object,omitempty"`
}

type AtContext struct {
	Context string `json:"@context,omitempty"`
}

type AtContextArray struct {
	Context []interface{} `json:"@context,omitempty"`
}

type AtContextString struct {
	Context string `json:"@context,omitempty"`
}

type ActorString struct {
	Actor string `json:"actor,omitempty"`
}

type ObjectArray struct {
	Object []ObjectBase `json:"object,omitempty"`
}

type Object struct {
	Object *ObjectBase `json:"object,omitempty"`
}

type ObjectString struct {
	Object string `json:"object,omitempty"`
}

type ToArray struct {
	To []string `json:"to,omitempty"`
}

type ToString struct {
	To string `json:"to,omitempty"`
}

type CcArray struct {
	Cc []string `json:"cc,omitempty"`
}

type CcOjectString struct {
	Cc string `json:"cc,omitempty"`
}

type Actor struct {
	Type              string        `json:"type,omitempty"`
	Id                string        `json:"id,omitempty"`
	Inbox             string        `json:"inbox,omitempty"`
	Outbox            string        `json:"outbox,omitempty"`
	Following         string        `json:"following,omitempty"`
	Followers         string        `json:"followers,omitempty"`
	Name              string        `json:"name,omitempty"`
	PreferredUsername string        `json:"preferredUsername,omitempty"`
	PublicKey         *PublicKeyPem `json:"publicKey,omitempty"`
	Summary           string        `json:"summary,omitempty"`
	Restricted        bool          `json:"restricted"`
}

type PublicKeyPem struct {
	Id           string `json:"id,omitempty"`
	Owner        string `json:"owner,omitempty"`
	PublicKeyPem string `json:"publicKeyPem,omitempty"`
}

type Activity struct {
	AtContext
	Type      string     `json:"type,omitempty"`
	Id        string     `json:"id,omitempty"`
	Actor     *Actor     `json:"actor,omitempty"`
	Name      string     `json:"name,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	To        []string   `json:"to,omitempty"`
	Cc        []string   `json:"cc,omitempty"`
	Published time.Time  `json:"published,omitempty"`
	Object    ObjectBase `json:"object,omitempty"`

	// Auth      string     `json:"auth,omitempty"`
	// Bto       []string   `json:"bto,omitempty"`
}

type ObjectBase struct {
	Type         string          `json:"type,omitempty"`
	Id           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Option       []string        `json:"-"`
	AttributedTo string          `json:"attributedTo,omitempty"`
	TripCode     string          `json:"tripcode,omitempty"`
	Actor        string          `json:"actor,omitempty"`
	Content      string          `json:"content,omitempty"`
	InReplyTo    []ObjectBase    `json:"inReplyTo,omitempty"`
	Preview      *ObjectBase     `json:"preview,omitempty"`
	Published    time.Time       `json:"published,omitempty"`
	Updated      *time.Time      `json:"updated,omitempty"`
	Object       *ObjectBase     `json:"object,omitempty"`
	Attachment   []ObjectBase    `json:"attachment,omitempty"`
	Replies      *CollectionBase `json:"replies,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Url          []ObjectBase    `json:"url,omitempty"`
	Href         string          `json:"href,omitempty"`
	To           []string        `json:"to,omitempty"`
	Cc           []string        `json:"cc,omitempty"`
	Bcc          string          `json:"Bcc,omitempty"`
	MediaType    string          `json:"mediatype,omitempty"`
	Size         int64           `json:"size,omitempty"`
	Sensitive    bool            `json:"sensitive,omitempty"`
	Sticky       bool            `json:"sticky,omitempty"`
	Locked       bool            `json:"locked,omitempty"`

	// Alias        string          `json:"alias,omitempty"`
	// Audience     string          `json:"audience,omitempty"`
	// Bto          []string        `json:"bto,omitempty"`
	// ContentHTML  template.HTML   `json:"contenthtml,omitempty"`
	// Deleted      string          `json:"deleted,omitempty"`
	// Duration     string          `json:"duration,omitempty"`
	// EndTime      string          `json:"endTime,omitempty"`
	// Generator    string          `json:"generator,omitempty"`
	// Icon         string          `json:"icon,omitempty"`
	// Image        string          `json:"image,omitempty"`
	// Location     string          `json:"location,omitempty"`
	// StartTime    string          `json:"startTime,omitempty"`
	// Tag          []ObjectBase    `json:"tag,omitempty"`
}

type CollectionBase struct {
	Actor        *Actor       `json:"actor,omitempty"`
	Summary      string       `json:"summary,omitempty"`
	Type         string       `json:"type,omitempty"`
	TotalItems   int          `json:"totalItems,omitempty"`
	TotalImgs    int          `json:"totalImgs,omitempty"`
	OrderedItems []ObjectBase `json:"orderedItems,omitempty"`
	Items        []ObjectBase `json:"items,omitempty"`
}

type Collection struct {
	AtContext
	CollectionBase
}

type ObjectBaseSortDesc []ObjectBase

func (a ObjectBaseSortDesc) Len() int { return len(a) }
func (a ObjectBaseSortDesc) Less(i, j int) bool {
	if a[i].Updated == nil && a[j].Updated == nil {
		return true
	} else if a[i].Updated == nil {
		return false
	} else if a[j].Updated == nil {
		return true
	}
	return a[i].Updated.After(*a[j].Updated)
}
func (a ObjectBaseSortDesc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type ObjectBaseSortAsc []ObjectBase

func (a ObjectBaseSortAsc) Len() int           { return len(a) }
func (a ObjectBaseSortAsc) Less(i, j int) bool { return a[i].Published.Before(a[j].Published) }
func (a ObjectBaseSortAsc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
