(*
Module: Wireguard
  Parses WireGuard tunnel configuration files (wg-quick / wg setconf).

Author: the go-augeas/augeas authors

About: Reference
  wg(8), wg-quick(8) - https://man7.org/linux/man-pages/man8/wg.8.html
  The configuration is an INI-like format with an [Interface] section and
  one or more [Peer] sections.

About: License
  This file is licensed under the LGPL v2+, like the rest of Augeas.

About: Lens Usage
  Sample usage of this lens in augtool

    > print /files/etc/wireguard/wg0.conf

About: Configuration files
  This lens applies to /etc/wireguard/ *.conf files.

About: Examples
  The tests at the end of this file document the supported syntax.
*)

module Wireguard =

autoload xfm

(* View: comment
     A "#" comment line *)
let comment = IniFile.comment "#" "#"

(* View: empty
     An empty line *)
let empty = Util.empty

(* View: eol *)
let eol = Util.eol

(* View: sep
     Key/value separator, "Key = Value", normalised to " = " *)
let sep = Sep.space_equal

(* View: entry
   A "Key = Value" line. The value is stored verbatim to the end of the
   line so shell hooks (PostUp/PostDown) that contain ';' and '#', base64
   keys ending in '=', and comma-separated AllowedIPs all round-trip. *)
let entry_kw = /[A-Za-z][A-Za-z0-9]*/
let entry = [ Util.indent . key entry_kw . sep . store Rx.space_in . eol ]

(* View: title
   Only [Interface] and [Peer] section headers are valid. *)
let title = IniFile.title /(Interface|Peer)/

(* View: record
   A section with its entries, comments and blank lines. *)
let record = IniFile.record title (entry|comment)

(* View: lns
   The Wireguard lens *)
let lns = IniFile.lns record comment

(* View: filter *)
let filter = incl "/etc/wireguard/*.conf"
           . Util.stdexcl

let xfm = transform lns filter

(* Test: Wireguard.lns
   A full client tunnel with one peer *)
let conf = "[Interface]
Address = 10.0.0.2/24
ListenPort = 51820
PrivateKey = SGVsbG9Xb3JsZFByaXZhdGVLZXlBQUFBQUFBQUFBQUE=
DNS = 1.1.1.1, 1.0.0.1

[Peer]
# main gateway
PublicKey = SGVsbG9Xb3JsZFB1YmxpY0tleUJCQkJCQkJCQkJCQg==
PresharedKey = SGVsbG9Xb3JsZFByZXNoYXJlZEtleUNDQ0NDQ0NDQz0=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:51820
PersistentKeepalive = 25
"

test Wireguard.lns get conf =
  { "Interface"
    { "Address" = "10.0.0.2/24" }
    { "ListenPort" = "51820" }
    { "PrivateKey" = "SGVsbG9Xb3JsZFByaXZhdGVLZXlBQUFBQUFBQUFBQUE=" }
    { "DNS" = "1.1.1.1, 1.0.0.1" }
    {  } }
  { "Peer"
    { "#comment" = "main gateway" }
    { "PublicKey" = "SGVsbG9Xb3JsZFB1YmxpY0tleUJCQkJCQkJCQkJCQg==" }
    { "PresharedKey" = "SGVsbG9Xb3JsZFByZXNoYXJlZEtleUNDQ0NDQ0NDQz0=" }
    { "AllowedIPs" = "0.0.0.0/0, ::/0" }
    { "Endpoint" = "vpn.example.com:51820" }
    { "PersistentKeepalive" = "25" } }

(* Test: round-trip an unmodified file *)
test Wireguard.lns put conf after
    set "/Peer/PersistentKeepalive" "25"
  = conf

(* Test: change the keepalive value and write it back *)
test Wireguard.lns put conf after
    set "/Peer/PersistentKeepalive" "15"
  = "[Interface]
Address = 10.0.0.2/24
ListenPort = 51820
PrivateKey = SGVsbG9Xb3JsZFByaXZhdGVLZXlBQUFBQUFBQUFBQUE=
DNS = 1.1.1.1, 1.0.0.1

[Peer]
# main gateway
PublicKey = SGVsbG9Xb3JsZFB1YmxpY0tleUJCQkJCQkJCQkJCQg==
PresharedKey = SGVsbG9Xb3JsZFByZXNoYXJlZEtleUNDQ0NDQ0NDQz0=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:51820
PersistentKeepalive = 15
"

(* Test: a PostUp hook containing ';' and shell must survive verbatim *)
let hooky = "[Interface]
Address = 10.0.0.1/24
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
"

test Wireguard.lns get hooky =
  { "Interface"
    { "Address" = "10.0.0.1/24" }
    { "PostUp" = "iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE" }
    { "PostDown" = "iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE" } }
