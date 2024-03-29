package db

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/simia-tech/crypt"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const SaltTable = "" +
	"................................" +
	".............../0123456789ABCDEF" +
	"GABCDEFGHIJKLMNOPQRSTUVWXYZabcde" +
	"fabcdefghijklmnopqrstuvwxyz....." +
	"................................" +
	"................................" +
	"................................" +
	"................................"

func CreateNameTripCode(input string, a *Acct) (string, string, error) {
	// TODO: Capcode support has been removed. Add it back in.

	tripSecure := regexp.MustCompile("##(.+)?")

	if tripSecure.MatchString(input) {
		chunck := tripSecure.FindString(input)
		chunck = strings.Replace(chunck, "##", "", 1)

		hash, err := TripCodeSecure(chunck)

		return tripSecure.ReplaceAllString(input, ""), "!!" + hash, wrapErr(err)
	}

	trip := regexp.MustCompile("#(.+)?")

	if trip.MatchString(input) {
		chunck := trip.FindString(input)
		chunck = strings.Replace(chunck, "#", "", 1)

		hash, err := TripCode(chunck)
		return trip.ReplaceAllString(input, ""), "!" + hash, wrapErr(err)
	}

	return input, "", nil
}

func TripCode(pass string) (string, error) {
	var salt [2]rune

	pass = TripCodeConvert(pass)
	s := []rune(pass + "H..")[1:3]

	for i, r := range s {
		salt[i] = rune(SaltTable[r%256])
	}

	enc, err := crypt.Crypt(pass, "$1$"+string(salt[:]))

	if err != nil {
		return "", wrapErr(err)
	}

	// normally i would just return error here but if the encrypt fails, this operation may fail and as a result cause a panic
	return enc[len(enc)-10:], nil
}

func TripCodeConvert(str string) string {
	var s bytes.Buffer

	transform.NewWriter(&s, japanese.ShiftJIS.NewEncoder()).Write([]byte(str))
	re := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)

	return re.Replace(s.String())
}

func TripCodeSecure(pass string) (string, error) {
	pass = TripCodeConvert(pass)
	enc, err := crypt.Crypt(pass, "$1$"+config.Salt)

	if err != nil {
		return "", wrapErr(err)
	}

	return enc[len(enc)-10:], nil
}
