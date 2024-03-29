package activitypub

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
)

func (obj ObjectBase) WantToCache(actor Actor) (bool, error) {
	reqActivity := Activity{Id: obj.Actor + "/followers"}
	objFollowers, err := reqActivity.GetCollection()

	if err != nil {
		return false, util.WrapError(err)
	}

	actorFollowing, err := actor.GetFollowing()

	if err != nil {
		return false, util.WrapError(err)
	}

	isOP, _ := obj.CheckIfOP()

	for _, e := range objFollowers.Items {
		if e.Id == actor.Id {
			return true, nil
		}

		for _, k := range actorFollowing {
			if e.Id == k.Id && !isOP && obj.InReplyTo[0].Id != "" {
				return true, nil
			}
		}
	}

	return false, nil
}

func (obj ObjectBase) CreateActivity(activityType string) (Activity, error) {
	var newActivity Activity

	actor, err := FingerActor(obj.Actor)
	if err != nil {
		return newActivity, util.WrapError(err)
	}

	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = &actor
	newActivity.Object = obj

	for _, e := range obj.To {
		if obj.Actor != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor != e {
			newActivity.Cc = append(newActivity.Cc, e)
		}
	}

	return newActivity, nil
}

func (obj ObjectBase) CheckIfOP() (bool, error) {
	var id string

	query := `select id from replies where inreplyto='' and id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		return false, nil
	}

	return true, nil
}

func (obj ObjectBase) GetOP() (string, error) {
	var id string

	query := `select id from replies where inreplyto='' and id in (select inreplyto from replies where id=$1)`

	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		return obj.Id, nil
	}

	return id, nil
}

func (obj ObjectBase) CreatePreview() *ObjectBase {
	var nPreview ObjectBase

	re := regexp.MustCompile(`/.+$`)
	mimetype := re.ReplaceAllString(obj.MediaType, "")

	if mimetype != "image" {
		return &nPreview
	}

	re = regexp.MustCompile(`.+/`)
	file := re.ReplaceAllString(obj.MediaType, "")
	href := util.GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = config.Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)
	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+href)

	if err := cmd.Run(); err != nil {
		// TODO: previously we would call CheckError here
		var preview ObjectBase
		return &preview
	}

	return &nPreview
}

func (obj ObjectBase) DeleteAndRepliesRequest() error {
	activity, err := obj.CreateActivity("Delete")

	if err != nil {
		return util.WrapError(err)
	}

	nObj, err := obj.GetCollectionFromPath()
	if err != nil {
		return util.WrapError(err)
	}

	activity.Actor.Id = nObj.OrderedItems[0].Actor
	activity.Object = nObj.OrderedItems[0]
	objActor, _ := GetActor(nObj.OrderedItems[0].Actor)
	followers, err := objActor.GetFollower()

	if err != nil {
		return util.WrapError(err)
	}
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := objActor.GetFollowing()
	if err != nil {
		return util.WrapError(err)
	}

	for _, e := range following {
		if !util.IsInStringArray(activity.To, e.Id) {
			activity.To = append(activity.To, e.Id)
		}
	}

	err = activity.Send()

	return util.WrapError(err)
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) DeleteAttachment() error {
	query := `delete from activitystream where id in (select attachment from activitystream where id=$1)`
	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `delete from cacheactivitystream where id in (select attachment from cacheactivitystream where id=$1)`
	_, err := config.DB.Exec(query, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) DeleteAttachmentFromFile() error {
	var href string

	query := `select href from activitystream where id in (select attachment from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)
	if href != "static/notfound.png" {
		if _, err := os.Stat(href); err != nil {
			return nil
		}
		return os.Remove(href)
	}

	return nil
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) DeletePreview() error {
	query := `delete from activitystream where id=$1`

	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `delete from cacheactivitystream where id in (select preview from cacheactivitystream where id=$1)`

	_, err := config.DB.Exec(query, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) DeletePreviewFromFile() error {
	var href string

	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)
	if href != "static/notfound.png" {
		if _, err := os.Stat(href); err != nil {
			return nil
		}
		return os.Remove(href)
	}

	return nil
}

func (obj ObjectBase) DeleteAll() error {
	if err := obj.DeleteReported(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeleteAttachment(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeletePreview(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.Delete(); err != nil {
		return util.WrapError(err)
	}

	return obj.DeleteRepliedTo()
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) Delete() error {
	query := `delete from activitystream where id=$1`
	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `delete from cacheactivitystream where id=$1`
	_, err := config.DB.Exec(query, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) DeleteInReplyTo() error {
	query := `delete from replies where id in (select id from replies where inreplyto=$1)`
	_, err := config.DB.Exec(query, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) DeleteRepliedTo() error {
	query := `delete from replies where id=$1`
	_, err := config.DB.Exec(query, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) DeleteRequest() error {
	activity, err := obj.CreateActivity("Delete")

	if err != nil {
		return util.WrapError(err)
	}

	nObj, err := obj.GetFromPath()

	if err != nil {
		return util.WrapError(err)
	}

	actor, err := FingerActor(nObj.Actor)

	if err != nil {
		return util.WrapError(err)
	}

	activity.Actor = &actor
	objActor, _ := GetActor(nObj.Actor)
	followers, err := objActor.GetFollower()

	if err != nil {
		return util.WrapError(err)
	}

	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := objActor.GetFollowing()
	if err != nil {
		return util.WrapError(err)
	}

	for _, e := range following {
		if !util.IsInStringArray(activity.To, e.Id) {
			activity.To = append(activity.To, e.Id)
		}
	}

	err = activity.Send()

	return util.WrapError(err)
}

func (obj ObjectBase) DeleteReported() error {
	query := `delete from reported where id=$1`
	_, err := config.DB.Exec(query, obj.Id)

	return util.WrapError(err)
}

func (obj ObjectBase) GetCollectionLocal() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var rows *sql.Rows
	var err error

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 and (type='Note' or type='Archive') union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 and (type='Note' or type='Archive')) as x`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nColl, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var actor Actor
		var post ObjectBase

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.WrapError(err)
		}

		post.Sticky, _ = post.IsSticky()
		post.Locked, _ = post.IsLocked()

		post.Actor = actor.Id

		if post.InReplyTo, err = post.GetInReplyTo(); err != nil {
			return nColl, util.WrapError(err)
		}

		if post.Replies, err = post.GetReplies(); err != nil {
			return nColl, util.WrapError(err)
		}

		if post.Replies != nil {
			var postCnt int
			var imgCnt int

			if postCnt, imgCnt, err = post.GetRepliesCount(); err != nil {
				return nColl, util.WrapError(err)
			}

			post.Replies.TotalItems += postCnt
			post.Replies.TotalImgs += imgCnt
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			if post.Preview, err = prev.GetPreview(); err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.Actor = &Actor{Id: obj.Id}

	nColl.OrderedItems = result

	return nColl, nil
}

func (obj ObjectBase) GetInReplyTo() ([]ObjectBase, error) {
	var result []ObjectBase

	query := `select inreplyto from replies where id =$1`
	rows, err := config.DB.Query(query, obj.Id)

	if err != nil {
		return result, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		if err := rows.Scan(&post.Id); err != nil {
			return result, util.WrapError(err)
		}

		result = append(result, post)
	}

	return result, nil
}

// TODO does attachemnts need to be an array in the activitypub structs?
func (obj ObjectBase) GetAttachment() ([]ObjectBase, error) {
	var attachment ObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`
	err := config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return []ObjectBase{attachment}, nil
}

func (obj ObjectBase) GetCollectionFromPath() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var post ObjectBase
	var actor Actor

	var attch ObjectBase

	var prev ObjectBase

	var err error

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id like $1 and (type='Note' or type='Archive') union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id like $1 and (type='Note' or type='Archive')) as x order by x.updated`
	if err = config.DB.QueryRow(query, obj.Id).Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
		return nColl, err
	}

	post.Sticky, _ = post.IsSticky()
	post.Locked, _ = post.IsLocked()

	post.Actor = actor.Id

	if post.InReplyTo, err = post.GetInReplyTo(); err != nil {
		return nColl, util.WrapError(err)
	}

	if post.Replies, err = post.GetReplies(); err != nil {
		return nColl, util.WrapError(err)
	}

	if attch.Id != "" {
		post.Attachment, err = attch.GetAttachment()
		if err != nil {
			return nColl, util.WrapError(err)
		}
	}

	if prev.Id != "" {
		if post.Preview, err = prev.GetPreview(); err != nil {
			return nColl, util.WrapError(err)
		}
	}

	result = append(result, post)

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.Actor = &Actor{Id: post.Actor}

	nColl.OrderedItems = result

	return nColl, nil
}

func (obj ObjectBase) GetFromPath() (ObjectBase, error) {
	var post ObjectBase

	var attch ObjectBase

	var prev ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`
	err := config.DB.QueryRow(query, obj.Id).Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &post.Actor)

	if err != nil {
		return post, util.WrapError(err)
	}

	post.Replies, err = post.GetReplies()
	if err != nil {
		return post, util.WrapError(err)
	}

	if post.Replies != nil {
		var postCnt int
		var imgCnt int

		postCnt, imgCnt, err = post.GetRepliesCount()
		if err != nil {
			return post, util.WrapError(err)
		}

		post.Replies.TotalItems += postCnt
		post.Replies.TotalImgs += imgCnt
	}

	if attch.Id != "" {
		post.Attachment, err = attch.GetAttachment()
		if err != nil {
			return post, util.WrapError(err)
		}
	}

	if prev.Id != "" {
		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return post, util.WrapError(err)
		}
	}

	return post, util.WrapError(err)
}

func (obj ObjectBase) GetPreview() (*ObjectBase, error) {
	var preview ObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published); err != nil {
		return nil, err
	}
	if preview.Id == "" {
		return nil, nil
	}
	return &preview, nil
}

func (obj ObjectBase) GetRepliesCount() (int, int, error) {
	var countId int
	var countImg int

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over() from (select id, attachment from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select id, attachment from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x`

	if err := config.DB.QueryRow(query, obj.Id).Scan(&countId, &countImg); err != nil {
		return 0, 0, nil
	}

	return countId, countImg, nil
}

func (obj ObjectBase) GetReplies() (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var rows *sql.Rows
	var err error

	query := `select x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nil, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)
		if err != nil {
			return nil, util.WrapError(err)
		}
		postCount++

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesReplies()

		if err != nil {
			return nil, util.WrapError(err)
		}

		if attch.Id != "" {
			attachCount++
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetRepliesLimit(limit int) (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var rows *sql.Rows
	var err error

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published desc limit $2`
	if rows, err = config.DB.Query(query, obj.Id, limit); err != nil {
		return nil, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nil, util.WrapError(err)
		}

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesReplies()

		if err != nil {
			return nil, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	sort.Sort(ObjectBaseSortAsc(result))

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetRepliesReplies() (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var err error
	var rows *sql.Rows

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nil, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {

		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)
		if err != nil {
			return nil, util.WrapError(err)
		}

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetType() (string, error) {
	var nType string

	query := `select type from activitystream where id=$1 union select type from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nType); err != nil {
		return "", nil
	}

	return nType, nil
}

func (obj ObjectBase) IsCached() (bool, error) {
	var nID string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nID); err != nil {
		return false, util.WrapError(err)
	}

	return true, nil
}

func (obj ObjectBase) IsLocal() (bool, error) {
	var nID string

	query := `select id from activitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nID); err != nil {
		return false, nil
	}

	return true, nil
}

func (obj ObjectBase) IsReplyInThread(id string) (bool, error) {
	reqActivity := Activity{Id: obj.InReplyTo[0].Id}
	coll, _, err := reqActivity.CheckValid()

	if err != nil {
		return false, util.WrapError(err)
	}

	for _, e := range coll.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true, nil
		}
	}

	return false, nil
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) MarkSensitive(sensitive bool) error {
	var query = `update activitystream set sensitive=$1 where id=$2`
	if _, err := config.DB.Exec(query, sensitive, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set sensitive=$1 where id=$2`
	_, err := config.DB.Exec(query, sensitive, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetAttachmentType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream  where id=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetAttachmentRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id in (select id from replies where inreplyto=$3))`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream where id in (select id from replies where inreplyto=$3))`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetPreviewType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetPreviewRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id in (select id from replies where inreplyto=$3))`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id in (select id from replies where inreplyto=$3))`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetType(_type string) error {
	if err := obj.SetAttachmentType(_type); err != nil {
		return util.WrapError(err)
	}

	if err := obj.SetPreviewType(_type); err != nil {
		return util.WrapError(err)
	}

	return obj._SetType(_type)
}

func (obj ObjectBase) _SetType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id=$3`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id=$3`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) SetRepliesType(_type string) error {
	if err := obj.SetAttachmentType(_type); err != nil {
		return util.WrapError(err)
	}

	if err := obj.SetPreviewType(_type); err != nil {
		return util.WrapError(err)
	}

	if err := obj._SetRepliesType(_type); err != nil {
		return util.WrapError(err)
	}

	if err := obj.SetAttachmentRepliesType(_type); err != nil {
		return util.WrapError(err)
	}

	if err := obj.SetPreviewRepliesType(_type); err != nil {
		return util.WrapError(err)
	}

	return obj.SetType(_type)
}

func (obj ObjectBase) _SetRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) TombstoneAttachment() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) TombstoneAttachmentReplies() error {
	var attachment ObjectBase

	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id); err != nil {
		return nil
	}

	if err := attachment.DeleteAttachmentFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := attachment.TombstoneAttachment(); err != nil {
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) TombstonePreview() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) TombstonePreviewReplies() error {
	var attachment ObjectBase

	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id); err != nil {
		return nil
	}

	if err := attachment.DeletePreviewFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := attachment.TombstonePreview(); err != nil {
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) Tombstone() error {
	if err := obj.DeleteReported(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.WrapError(err)
	}

	return obj._Tombstone()
}

func (obj ObjectBase) _Tombstone() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id=$2`
	if _, err := config.DB.Exec(query, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='',  deleted=$1 where id=$2`
	_, err := config.DB.Exec(query, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) TombstoneReplies() error {
	if err := obj.DeleteReported(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.WrapError(err)
	}

	if err := obj._TombstoneReplies(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstoneAttachmentReplies(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstonePreviewReplies(); err != nil {
		return util.WrapError(err)
	}

	return obj.Tombstone()
}

func (obj ObjectBase) _TombstoneReplies() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`
	if _, err := config.DB.Exec(query, datetime, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`
	_, err := config.DB.Exec(query, datetime, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) UpdateType(_type string) error {
	query := `update activitystream set type=$2 where id=$1 and type !='Tombstone'`
	if _, err := config.DB.Exec(query, obj.Id, _type); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set type=$2 where id=$1 and type !='Tombstone'`
	_, err := config.DB.Exec(query, obj.Id, _type)
	return util.WrapError(err)
}

func (obj ObjectBase) UpdatePreview(preview string) error {
	query := `update activitystream set preview=$1 where attachment=$2`
	_, err := config.DB.Exec(query, preview, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) Write() (ObjectBase, error) {
	id, err := util.CreateUniqueID(obj.Actor)
	if err != nil {
		return obj, util.WrapError(err)
	}

	obj.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
	if len(obj.Attachment) > 0 {
		now := time.Now().UTC()
		if obj.Preview.Href != "" {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, util.WrapError(err)
			}

			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Preview.Published = now
			obj.Preview.Updated = &now
			obj.Preview.AttributedTo = obj.Id
			if err := obj.Preview.WritePreview(); err != nil {
				return obj, util.WrapError(err)
			}
		}
		for i := range obj.Attachment {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, util.WrapError(err)
			}

			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Attachment[i].Published = now
			obj.Attachment[i].Updated = &now
			obj.Attachment[i].AttributedTo = obj.Id
			obj.Attachment[i].WriteAttachment()
			obj.WriteWithAttachment(obj.Attachment[i])
		}
	} else {
		if err := obj._Write(); err != nil {
			return obj, util.WrapError(err)
		}
	}

	err = obj.WriteReply()

	return obj, util.WrapError(err)
}

func (obj ObjectBase) _Write() error {

	query := `insert into activitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	return util.WrapError(err)
}

func (obj ObjectBase) WriteAttachment() error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)

	return util.WrapError(err)
}

func (obj ObjectBase) WriteAttachmentCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) _WriteCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) WriteCacheWithAttachment(attachment ObjectBase) error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) WritePreview() error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return util.WrapError(err)
}

func (obj ObjectBase) WritePreviewCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	err := config.DB.QueryRow(query, obj.Id).Scan(&id)
	if err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return util.WrapError(err)
	}

	return nil
}

func (obj ObjectBase) WriteReply() error {
	for i, e := range obj.InReplyTo {
		if isOP, err := obj.CheckIfOP(); !isOP && i == 0 {
			var nObj ObjectBase
			nObj.Id = e.Id

			nType, err := nObj.GetType()
			if err != nil {
				return util.WrapError(err)
			}

			if nType == "Archive" {
				if err := obj.UpdateType("Archive"); err != nil {
					return util.WrapError(err)
				}
			}
		} else if err != nil {
			return util.WrapError(err)
		}

		var id string

		query := `select id from replies where id=$1 and inreplyto=$2`
		if err := config.DB.QueryRow(query, obj.Id, e.Id).Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`
			if _, err := config.DB.Exec(query, obj.Id, e.Id); err != nil {
				return util.WrapError(err)
			}
		}

		update := true
		for _, o := range obj.Option {
			if o == "sage" || o == "nokosage" {
				update = false
				break
			}
		}

		if update {
			if err := e.WriteUpdate(obj.Published); err != nil {
				return util.WrapError(err)
			}
		}
	}

	if len(obj.InReplyTo) == 0 {
		var id string

		query := `select id from replies where id=$1 and inreplyto=''`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`
			if _, err := config.DB.Exec(query, obj.Id, ""); err != nil {
				return util.WrapError(err)
			}
		}
	}

	return nil
}

func (obj ObjectBase) WriteCache() (ObjectBase, error) {
	if isBlacklisted, err := util.IsPostBlacklist(obj.Content); err != nil || isBlacklisted {
		log.Println("Blacklist post blocked")
		return obj, util.WrapError(err)
	}

	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			obj.Preview.WritePreviewCache()
		}

		for i := range obj.Attachment {
			obj.Attachment[i].WriteAttachmentCache()
			obj.WriteCacheWithAttachment(obj.Attachment[i])
		}
	} else {
		obj._WriteCache()
	}

	obj.WriteReply()

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			e.WriteCache()
		}
	}

	return obj, nil
}

func (obj ObjectBase) WriteUpdate(updated time.Time) error {
	query := `update activitystream set updated=$1 where id=$2`
	if _, err := config.DB.Exec(query, updated, obj.Id); err != nil {
		return util.WrapError(err)
	}

	query = `update cacheactivitystream set updated=$1 where id=$2`
	_, err := config.DB.Exec(query, updated, obj.Id)
	return util.WrapError(err)
}

func (obj ObjectBase) WriteWithAttachment(attachment ObjectBase) {

	query := `insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	if e != nil {
		log.Println("error inserting new activity with attachment")
		panic(e)
	}
}

func (obj ObjectBase) MarkSticky(actorID string) error {
	var count int

	var query = `select count(id) from replies where inreplyto='' and id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return util.WrapError(err)
	}

	if count == 1 {
		var nCount int
		query = `select count(activity_id) from sticky where activity_id=$1`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&nCount); err != nil {
			return util.WrapError(err)
		}

		if nCount > 0 {
			query = `delete from sticky where activity_id=$1`
			if _, err := config.DB.Exec(query, obj.Id); err != nil {
				return util.WrapError(err)
			}
		} else {
			query = `insert into sticky (actor_id, activity_id) values ($1, $2)`
			if _, err := config.DB.Exec(query, actorID, obj.Id); err != nil {
				return util.WrapError(err)
			}
		}
	}

	return nil
}

func (obj ObjectBase) MarkLocked(actorID string) error {
	var count int

	var query = `select count(id) from replies where inreplyto='' and id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return util.WrapError(err)
	}

	if count == 1 {
		var nCount int

		query = `select count(activity_id) from locked where activity_id=$1`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&nCount); err != nil {
			return util.WrapError(err)
		}

		if nCount > 0 {
			query = `delete from locked where activity_id=$1`
			if _, err := config.DB.Exec(query, obj.Id); err != nil {
				return util.WrapError(err)
			}
		} else {
			query = `insert into locked (actor_id, activity_id) values ($1, $2)`
			if _, err := config.DB.Exec(query, actorID, obj.Id); err != nil {
				return util.WrapError(err)
			}
		}
	}

	return nil
}

func (obj ObjectBase) IsSticky() (bool, error) {
	var count int

	query := `select count(activity_id) from sticky where activity_id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return false, util.WrapError(err)
	}

	if count != 0 {
		return true, nil
	}

	return false, nil
}

func (obj ObjectBase) IsLocked() (bool, error) {
	var count int

	query := `select count(activity_id) from locked where activity_id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return false, util.WrapError(err)
	}

	if count != 0 {
		return true, nil
	}

	return false, nil
}

func collectTo() ([]string, error) {
	// This is a hack to prevent recursive dependencies

	rows, err := config.DB.Query(`select email from accounts where type >= 1 and email is not null`)
	if err != nil {
		return nil, util.WrapError(err)
	}
	defer rows.Close()

	mails := []string{}

	for rows.Next() {
		v := ""
		if err := rows.Scan(&v); err != nil {
			return mails, util.WrapError(err)
		}

		mails = append(mails, v)
	}

	return mails, nil
}

func (obj ObjectBase) SendEmailNotify() {
	if setup := config.IsEmailSetup(); !setup {
		return
	}

	actor, err := GetActorFromDB(obj.Actor)
	if err != nil {
		log.Println(util.WrapError(err))
		return
	}

	to, err := collectTo()
	if err != nil {
		log.Println(util.WrapError(err))
		return
	}

	irt := ""
	if len(obj.InReplyTo) > 0 {
		irt = obj.InReplyTo[0].Id
	}

	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", config.SiteEmailFrom),
		fmt.Sprintf("To: %s", strings.Join(to, ", ")),
		fmt.Sprintf("Subject: New post: %s/%s/%s", config.Domain, actor.Name, util.ShortURL(actor.Outbox, obj.Id)),
		"",
		fmt.Sprintf("Name: %s %s", obj.AttributedTo, obj.TripCode),
		fmt.Sprintf("InReplyTo: %s", irt),
		fmt.Sprintf("Subject: %s", obj.Name),
		"",
		obj.Content,
	}, "\n")

	domain, _, _ := strings.Cut(config.SiteEmailSMTP, ":")
	err = smtp.SendMail(config.SiteEmailSMTP,
		smtp.PlainAuth("", config.SiteEmailUser, config.SiteEmailPassword, domain),
		config.SiteEmailFrom, to, []byte(msg))

	if err != nil {
		log.Println(util.WrapError(err))
	}
}
