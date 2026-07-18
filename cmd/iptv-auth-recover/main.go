package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

import "iptv/internal/authrecover"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	set := flag.NewFlagSet("iptv-auth-recover", flag.ContinueOnError)
	authenticator := set.String("auth", "", "hex Authenticator from the captured POST body")
	token := set.String("token", "", "EncryptToken passed to CTCGetAuthInfo")
	userID := set.String("user", "", "IPTV UserID")
	stbID := set.String("stbid", "", "optional STBID constraint")
	ip := set.String("ip", "", "optional embedded STB IP constraint")
	mac := set.String("mac", "", "optional embedded STB MAC constraint")
	digits := set.Int("digits", 7, "numeric business-password length")
	if err := set.Parse(args); err != nil {
		return err
	}
	result, err := authrecover.RecoverNumeric(context.Background(), authrecover.Sample{
		Authenticator: *authenticator,
		EncryptToken:  *token,
		UserID:        *userID,
		STBID:         *stbID,
		IP:            *ip,
		MAC:           *mac,
	}, *digits)
	if err != nil {
		return err
	}
	fmt.Println("matching DES-equivalent key found")
	fmt.Println("usable equivalent key:", result.EquivalentKey)
	fmt.Println("equivalent original pattern:", result.KeyPattern)
	fmt.Println("embedded random:", result.Random)
	if result.Reserved == "" {
		fmt.Println("reserved field: empty")
	} else {
		fmt.Println("reserved field:", result.Reserved)
	}
	if *stbID == "" {
		fmt.Println("embedded STBID:", result.STBID)
	}
	if *ip == "" {
		fmt.Println("embedded IP:", result.IP)
	}
	if *mac == "" {
		fmt.Println("embedded MAC:", result.MAC)
	}
	return nil
}
