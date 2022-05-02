package routes

import (
	"io/ioutil"
	"net/http"
	"time"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2"
)

func Media(c *fiber.Ctx) error {
	if c.Query("hash") != "" {
		return RouteImages(c, c.Query("hash"))
	}

	return c.SendStatus(404)
}

func RouteImages(ctx *fiber.Ctx, media string) error {
	req, err := http.NewRequest("GET", config.MediaHashs[media], nil)
	if err != nil {
		return err
	}

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fileBytes, err := ioutil.ReadFile("./static/notfound.png")
		if err != nil {
			return err
		}

		_, err = ctx.Write(fileBytes)
		return err
	}

	body, _ := ioutil.ReadAll(resp.Body)
	for name, values := range resp.Header {
		for _, value := range values {
			ctx.Append(name, value)
		}
	}

	return ctx.Send(body)
}
