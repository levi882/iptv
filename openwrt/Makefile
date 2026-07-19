include $(TOPDIR)/rules.mk

PKG_NAME:=iptv-refresh
PKG_VERSION:=0.1.0
PKG_RELEASE:=14
PKG_LICENSE:=Proprietary

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=iptv
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/iptv-refresh
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)-r$(PKG_RELEASE)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/iptv-refresh
  SECTION:=net
  CATEGORY:=Network
  TITLE:=HBIPTV playlist refresh service
  DEPENDS:=$(GO_ARCH_DEPENDS) +tcpdump +ca-bundle
endef

define Package/iptv-refresh/description
  Captures HBIPTV STB credentials, authenticates with the provider portal,
  generates M3U/EPG/logo outputs, and exposes a local refresh HTTP API.
endef

define Package/iptv-refresh/conffiles
/etc/config/iptv-refresh
/etc/iptv-refresh/hb.env
/etc/iptv-refresh/token
endef

define Package/iptv-refresh/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT:-}" ] && exit 0

token_file=/etc/iptv-refresh/token
token="$$(head -n 1 "$${token_file}" 2>/dev/null || true)"
if [ -z "$${token}" ] || [ "$${token}" = "change-me" ]; then
	random_file="$${token_file}.random.$$$$"
	umask 077
	if ! dd if=/dev/urandom of="$${random_file}" bs=32 count=1 2>/dev/null; then
		rm -f "$${random_file}"
		echo "ERROR: unable to generate API token" >&2
		exit 1
	fi
	token="$$(sha256sum "$${random_file}")"
	rm -f "$${random_file}"
	token="$${token%% *}"
	[ "$${#token}" -eq 64 ] || {
		echo "ERROR: generated API token has an invalid length" >&2
		exit 1
	}
	printf '%s\n' "$${token}" > "$${token_file}"
	chmod 600 "$${token_file}"
fi
listen_port="$$(uci -q get iptv-refresh.main.listen_port 2>/dev/null || true)"
if [ "$${listen_port}" = "9099" ]; then
	uci set iptv-refresh.main.listen_port='9100'
	uci commit iptv-refresh
fi
unset token random_file listen_port
exit 0
endef

define Package/iptv-refresh/postrm
#!/bin/sh
[ -n "$${IPKG_INSTROOT:-}" ] && exit 0
[ -x /usr/bin/iptv-refresh ] && exit 0
rm -f /etc/iptv-refresh/nginx.d/auth.conf /etc/nginx/conf.d/iptv-refresh.locations
rmdir /etc/iptv-refresh/nginx.d 2>/dev/null || true
for backup in /etc/nginx/conf.d/*.locations.iptv-refresh-legacy; do
	[ -e "$${backup}" ] || continue
	original="$${backup%.iptv-refresh-legacy}"
	[ -e "$${original}" ] || mv "$${backup}" "$${original}"
done
if [ -x /etc/init.d/nginx ] && /etc/init.d/nginx running >/dev/null 2>&1; then
	/etc/init.d/nginx reload >/dev/null 2>&1 || true
fi
exit 0
endef

define Build/Prepare
	rm -rf $(PKG_BUILD_DIR)
	$(INSTALL_DIR) $(PKG_BUILD_DIR)
	$(CP) ./src/. $(PKG_BUILD_DIR)/
endef

define Package/iptv-refresh/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(GO_PKG_BUILD_BIN_DIR)/iptv-refresh $(1)/usr/bin/iptv-refresh
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/iptv-refresh.init $(1)/etc/init.d/iptv-refresh
	$(INSTALL_DIR) $(1)/usr/libexec
	$(INSTALL_BIN) ./files/iptv-refresh-nginx-config $(1)/usr/libexec/iptv-refresh-nginx-config
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/iptv-refresh.uci $(1)/etc/config/iptv-refresh
	$(INSTALL_DIR) $(1)/etc/iptv-refresh
	$(INSTALL_CONF) ./files/hb.env $(1)/etc/iptv-refresh/hb.env
	$(INSTALL_CONF) ./files/token $(1)/etc/iptv-refresh/token
endef

$(eval $(call GoBinPackage,iptv-refresh))
$(eval $(call BuildPackage,iptv-refresh))
