package main

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/routes"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/template/html"
)

func main() {
	Init()

	defer db.Close()

	// Routing and templates
	template := html.New("./views", ".html")

	routes.TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName:      "FChannel",
		Views:        template,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ServerHeader: "FChannel/" + config.InstanceName,
	})

	app.Use(logger.New())

	app.Static("/static", "./views")
	app.Static("/public", "./public")

	if config.Debug == "yes" {
		log.Printf("pprof enabled!")
		app.Use(pprof.New())
	}

	// Authentication middleware
	// Creates the `acct` local if the session is good
	app.Use(func(ctx *fiber.Ctx) error {
		cookie := ctx.Cookies("session")
		var a db.Acct
		var err error
		if cookie == "" {
			// Try Authorization header
			cookie = ctx.Get("Authorization")
			if cookie == "" || !strings.HasPrefix(cookie, "Bearer ") {
				goto done
			}
			cookie = strings.TrimPrefix(cookie, "Bearer ")
		}

		a, err = db.LoginSession(cookie)
		if err != nil {
			// TODO: Drop cookie
			goto done
		}

		ctx.Locals("acct", &a)
	done:
		return ctx.Next()
	})

	// Main actor
	app.Get("/", routes.Index)
	app.Post("/inbox", routes.Inbox)
	app.Post("/outbox", routes.Outbox)
	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)

	// Admin routes
	app.All("/"+config.Key+"/", routes.AdminIndex)
	app.Post("/"+config.Key+"/verify", routes.AdminVerify)
	app.All("/"+config.Key+"/follow", routes.AdminFollow)
	app.Post("/"+config.Key+"/addboard", routes.AdminAddBoard)
	app.Post("/"+config.Key+"/newspost", routes.NewsPost)
	app.Get("/"+config.Key+"/newsdelete/:ts", routes.NewsDelete)
	app.Post("/"+config.Key+"/adduser", routes.AdminAddUser)
	app.Post("/"+config.Key+"/deluser", routes.AdminDeleteUser)
	app.Get("/"+config.Key+"/chpasswd", routes.AdminChangePasswd)
	app.Post("/"+config.Key+"/chpasswd", routes.AdminChangePasswd)
	app.Post("/"+config.Key+"/blotter", routes.AdminSetBlotter)
	app.Post("/"+config.Key+"/lock", routes.AdminSetLocked)
	app.Post("/"+config.Key+"/:actor/editsummary", routes.AdminEditSummary)
	app.All("/"+config.Key+"/:actor/follow", routes.AdminFollow)
	app.Get("/"+config.Key+"/:actor", routes.AdminActorIndex)

	// News routes
	app.Get("/news/:ts", routes.NewsGet)
	app.Get("/news", routes.NewsGetAll)

	// Board managment
	app.Get("/banmedia", routes.BoardBanMedia)
	app.Get("/delete", routes.BoardDelete)
	app.Get("/deleteattach", routes.BoardDeleteAttach)
	app.Get("/marksensitive", routes.BoardMarkSensitive)
	app.Get("/addtoindex", routes.BoardAddToIndex)
	app.Get("/poparchive", routes.BoardPopArchive)
	app.Get("/autosubscribe", routes.BoardAutoSubscribe)
	app.All("/blacklist", routes.BoardBlacklist)
	app.All("/report", routes.ReportPost)
	app.Get("/make-report", routes.ReportGet)
	app.Get("/sticky", routes.Sticky)
	app.Get("/lock", routes.Lock)

	// Webfinger routes
	app.Get("/.well-known/webfinger", routes.Webfinger)

	// API routes
	app.Get("/api/media", routes.Media)

	// Board actor routes
	app.Post("/post", routes.MakeActorPost)
	app.Get("/:actor/catalog", routes.ActorCatalog)
	app.Post("/:actor/inbox", routes.ActorInbox)
	app.Get("/:actor/outbox", routes.GetActorOutbox)
	app.Get("/:actor/following", routes.ActorFollowing)
	app.Get("/:actor/followers", routes.ActorFollowers)
	app.Get("/:actor/archive", routes.ActorArchive)
	app.Get("/:actor", routes.ActorPosts)
	app.Get("/:actor/:post", routes.ActorPost)

	if err := db.PrintAdminAuth(); err != nil {
		panic(err)
	}

	app.Listen(config.Port)
}

func Init() {
	var actor activitypub.Actor
	var err error

	rand.Seed(time.Now().UnixNano())

	if err = util.CreatedNeededDirectories(); err != nil {
		log.Fatal(err)
	}

	if err = db.Connect(); err != nil {
		log.Fatal(err)
	}

	if err = db.InitInstance(); err != nil {
		log.Fatal(err)
	}

	if actor, err = activitypub.GetActorFromDB(config.Domain); err != nil {
		log.Fatal(err)
	}

	if activitypub.FollowingBoards, err = actor.GetFollowing(); err != nil {
		log.Fatal(err)
	}

	if activitypub.Boards, err = activitypub.GetBoardCollection(); err != nil {
		log.Fatal(err)
	}

	if config.Key == "" {
		if config.Key, err = util.CreateKey(32); err != nil {
			log.Fatal(err)
		}
	}

	if err = util.LoadThemes(); err != nil {
		log.Fatal(err)
	}

	go activitypub.StartupArchive()

	go func() {
		if err := db.MakeCaptchas(); err != nil {
			log.Printf("Failed to create captchas: %v", err)
		}
	}()
}
