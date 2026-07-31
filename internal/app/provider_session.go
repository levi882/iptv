package app

import (
	"fmt"
	"slices"
	"strings"

	"iptv/internal/config"
	"iptv/internal/portal"
)

func resolveProviderMetadata(settings Settings, creds config.Env) (Settings, error) {
	settings.TokenServer = resolveValue(settings.TokenServer, creds["PROVIDER_TOKEN_SERVER"], "")
	settings.PlatformOrigin = resolveValue(settings.PlatformOrigin, creds["PROVIDER_PLATFORM_ORIGIN"], "")
	settings.EPGEntry = resolveValue(settings.EPGEntry, creds["PROVIDER_EPG_ENTRY"], "")
	settings.EASIP = resolveValue(settings.EASIP, creds["PROVIDER_EASIP"], "")
	settings.NetworkID = resolveValue(settings.NetworkID, creds["PROVIDER_NETWORKID"], "")
	settings.CityCode = resolveValue(settings.CityCode, creds["PROVIDER_CITYCODE"], "")
	settings.STBType = resolveValue(settings.STBType, creds["PROVIDER_STB_TYPE"], "")
	settings.PRMID = resolveValue(settings.PRMID, creds["PROVIDER_PRMID"], "")
	settings.DRMSupplier = resolveValue(settings.DRMSupplier, creds["PROVIDER_DRM_SUPPLIER"], "")
	settings.UserAgent = resolveValue(settings.UserAgent, creds["PROVIDER_USER_AGENT"], "")

	missing := []string{}
	for key, value := range map[string]string{
		"TOKEN_SERVER":    settings.TokenServer,
		"PLATFORM_ORIGIN": settings.PlatformOrigin,
		"EPG_ENTRY":       settings.EPGEntry,
		"EASIP":           settings.EASIP,
		"NETWORKID":       settings.NetworkID,
		"STB_TYPE":        settings.STBType,
	} {
		if value == "" || strings.EqualFold(value, "auto") {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return Settings{}, fmt.Errorf("provider metadata missing (%s); recapture credentials or configure the provider environment", strings.Join(missing, ", "))
	}
	return settings, nil
}

func newProviderClient(settings Settings) (*portal.Client, error) {
	return portal.New(portal.Config{
		TokenServer: settings.TokenServer, PlatformOrigin: settings.PlatformOrigin, EPGEntry: settings.EPGEntry,
		EPGFallbacks: settings.EPGFallbacks, EASIP: settings.EASIP, NetworkID: settings.NetworkID, CityCode: settings.CityCode,
		UserAgent: settings.UserAgent, BindInterface: settings.BindInterface, BindSourceIP: settings.BindSourceIP, Timeout: settings.ProviderTimeout,
	})
}

func providerCredentials(settings Settings, creds config.Env) portal.Credentials {
	return portal.Credentials{
		UserID: creds["PROVIDER_USER_ID"], STBID: creds["PROVIDER_STBID"], Authenticator: creds["PROVIDER_AUTHENTICATOR"],
		STBInfo: creds["PROVIDER_STBINFO"], UserToken: creds["PROVIDER_USER_TOKEN"], STBType: settings.STBType,
		PRMID: settings.PRMID, DRMSupplier: settings.DRMSupplier,
	}
}
