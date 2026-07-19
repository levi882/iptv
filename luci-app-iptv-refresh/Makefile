include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-iptv-refresh
PKG_VERSION:=0.1.0
PKG_RELEASE:=17
PKG_PO_VERSION:=$(PKG_VERSION)-r$(PKG_RELEASE)
PKG_LICENSE:=Apache-2.0
PKG_LICENSE_FILES:=LICENSE

LUCI_TITLE:=LuCI support for IPTV Refresh
LUCI_DEPENDS:=+luci-base +rpcd-mod-file +iptv-refresh
LUCI_PKGARCH:=all
LUCI_MAINTAINER:=IPTV Refresh maintainers
LUCI_DESCRIPTION:=LuCI status, service control, configuration, log, refresh, and playlist interface for the iptv-refresh service.

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt package scanner signature
