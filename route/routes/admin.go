package routes

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/route"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func AdminVerify(ctx *fiber.Ctx) error {
	identifier := ctx.FormValue("id")
	code := ctx.FormValue("code")

	var verify util.Verify
	verify.Identifier = identifier
	verify.Code = code

	j, _ := json.Marshal(&verify)

	req, err := http.NewRequest("POST", config.Domain+"/auth", bytes.NewBuffer(j))

	if err != nil {
		return util.MakeError(err, "AdminVerify")
	}

	req.Header.Set("Content-Type", config.ActivityStreams)

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return util.MakeError(err, "AdminVerify")
	}

	defer resp.Body.Close()

	rBody, _ := ioutil.ReadAll(resp.Body)

	body := string(rBody)

	if resp.StatusCode != 200 {
		return ctx.Redirect("/"+config.Key, http.StatusPermanentRedirect)
	}

	ctx.Cookie(&fiber.Cookie{
		Name:    "session_token",
		Value:   body + "|" + verify.Code,
		Expires: time.Now().UTC().Add(60 * 60 * 48 * time.Second),
	})

	return ctx.Redirect("/", http.StatusSeeOther)
}

// TODO remove this route it is mostly unneeded
func AdminAuth(ctx *fiber.Ctx) error {
	var verify util.Verify

	err := json.Unmarshal(ctx.Body(), &verify)

	if err != nil {
		return util.MakeError(err, "AdminAuth")
	}

	v, _ := util.GetVerificationByCode(verify.Code)

	if v.Identifier == verify.Identifier {
		_, err := ctx.Write([]byte(v.Board))
		return util.MakeError(err, "AdminAuth")
	}

	ctx.Response().Header.SetStatusCode(http.StatusBadRequest)
	_, err = ctx.Write([]byte(""))

	return util.MakeError(err, "AdminAuth")
}

func AdminIndex(ctx *fiber.Ctx) error {
	id, _ := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	if id == "" || (id != actor.Id && id != config.Domain) {
		return ctx.Render("verify", fiber.Map{})
	}

	actor, err := activitypub.GetActor(config.Domain)

	if err != nil {
		return util.MakeError(err, "AdminIndex")
	}

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()
	follower, _ := reqActivity.GetCollection()

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	var adminData route.AdminPage
	adminData.Following = following
	adminData.Followers = followers
	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	adminData.Title = actor.Name + " Admin page"

	adminData.Boards = webfinger.Boards

	adminData.Board.Post.Actor = actor.Id

	adminData.PostBlacklist, _ = util.GetRegexBlacklist()

	adminData.Themes = &config.Themes

	return ctx.Render("admin", fiber.Map{
		"page": adminData,
	})
}

func AdminFollow(ctx *fiber.Ctx) error {
	follow := ctx.FormValue("follow")
	actorId := ctx.FormValue("actor")

	actor := activitypub.Actor{Id: actorId}
	followActivity, _ := actor.MakeFollowActivity(follow)

	objActor := activitypub.Actor{Id: followActivity.Object.Actor}

	if isLocal, _ := objActor.IsLocal(); !isLocal && followActivity.Actor.Id == config.Domain {
		_, err := ctx.Write([]byte("main board can only follow local boards. Create a new board and then follow outside boards from it."))
		return util.MakeError(err, "AdminIndex")
	}

	if actor, _ := activitypub.FingerActor(follow); actor.Id != "" {
		if err := followActivity.MakeRequestOutbox(); err != nil {
			return util.MakeError(err, "AdminFollow")
		}
	}

	var redirect string
	actor, _ = webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminAddBoard(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)

	if hasValidation := actor.HasValidation(ctx); !hasValidation {
		return nil
	}

	var newActorActivity activitypub.Activity
	var board activitypub.Actor

	var restrict bool
	if ctx.FormValue("restricted") == "True" {
		restrict = true
	} else {
		restrict = false
	}

	board.Name = ctx.FormValue("name")
	board.PreferredUsername = ctx.FormValue("prefname")
	board.Summary = ctx.FormValue("summary")
	board.Restricted = restrict

	newActorActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActorActivity.Type = "New"

	var nobj activitypub.ObjectBase
	newActorActivity.Actor = &actor
	newActorActivity.Object = &nobj

	newActorActivity.Object.Alias = board.Name
	newActorActivity.Object.Name = board.PreferredUsername
	newActorActivity.Object.Summary = board.Summary
	newActorActivity.Object.Sensitive = board.Restricted

	newActorActivity.MakeRequestOutbox()
	return ctx.Redirect("/"+config.Key, http.StatusSeeOther)
}

func AdminPostNews(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin post news")
}

func AdminNewsDelete(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin news delete")
}

func AdminActorIndex(ctx *fiber.Ctx) error {
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()

	reqActivity.Id = actor.Followers
	follower, _ := reqActivity.GetCollection()

	reqActivity.Id = actor.Id + "/reported"
	reported, _ := activitypub.GetActorCollectionReq(reqActivity.Id)

	var following []string
	var followers []string
	var reports []db.Report

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	for _, e := range reported.Items {
		var r db.Report
		r.Count = int(e.Size)
		r.ID = e.Id
		r.Reason = e.Content
		reports = append(reports, r)
	}

	localReports, _ := db.GetLocalReport(actor.Name)

	for _, e := range localReports {
		var r db.Report
		r.Count = e.Count
		r.ID = e.ID
		r.Reason = e.Reason
		reports = append(reports, r)
	}

	var data route.AdminPage
	data.Following = following
	data.Followers = followers
	data.Reported = reports
	data.Domain = config.Domain
	data.IsLocal, _ = actor.IsLocal()

	data.Title = "Manage /" + actor.Name + "/"
	data.Boards = webfinger.Boards
	data.Board.Name = actor.Name
	data.Board.Actor = actor
	data.Key = config.Key
	data.Board.TP = config.TP

	data.Board.Post.Actor = actor.Id

	data.AutoSubscribe, _ = actor.GetAutoSubscribe()

	data.Themes = &config.Themes

	data.RecentPosts, _ = actor.GetRecentPosts()

	if cookie := ctx.Cookies("theme"); cookie != "" {
		data.ThemeCookie = cookie
	}

	return ctx.Render("manage", fiber.Map{
		"page": data,
	})
}