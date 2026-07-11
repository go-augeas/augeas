(*
Module: Unbound
  Parses the configuration file of the Unbound DNS resolver
  (unbound.conf).

Author: the go-augeas/augeas authors

About: Reference
  unbound.conf(5) - https://unbound.docs.nlnetlabs.nl/en/latest/manpages/unbound.conf.html
  The file is a set of colon-terminated clauses (`server:`, `forward-zone:`,
  `remote-control:`, ...) each holding indented `key: value` options, plus
  top-level `include:` directives and `#` comments. Many keys repeat within a
  clause (interface:, access-control:, local-data:, forward-addr:, ...) and
  their order and multiplicity are significant, so they are modelled as an
  ordered list of nodes rather than a map that would collapse duplicates.

About: License
  This file is licensed under the LGPL v2+, like the rest of Augeas.

About: Lens Usage
  Sample usage of this lens in augtool

    > print /files/etc/unbound/unbound.conf

About: Configuration files
  This lens applies to /etc/unbound/unbound.conf and the drop-in files under
  /etc/unbound/unbound.conf.d/.

About: Examples
  The tests at the end of this file document the supported syntax.
*)

module Unbound =

autoload xfm

(* View: comment
     A "#" comment line *)
let comment = Util.comment

(* View: empty
     An empty line *)
let empty = Util.empty

(* View: eol *)
let eol = Util.eol

(* View: indent *)
let indent = Util.indent

(* View: entry
   A "key: value" option line inside a clause. Clause bodies are indented, so
   an option line carries mandatory leading whitespace; this is what keeps a
   top-level `include:` directive (written at column 0) from being swallowed
   as a member of the preceding clause. The value is stored verbatim to the
   end of the line so quoted strings with spaces (local-data, local-zone),
   IP@port (forward-addr), CIDR + action (access-control) and bare tokens all
   round-trip. The "key:" separator is normalised to "key: ". *)
let entry_indent = del /[ \t]+/ "    "
let entry_kw = /[a-z][a-z0-9-]*/
let entry = [ entry_indent . key entry_kw
            . del /:[ \t]*/ ": " . store Rx.space_in . eol ]

(* View: include
   A top-level "include:" (or "include-toplevel:") directive, at column 0. *)
let incl_kw = /include(-toplevel)?/
let include = [ key incl_kw
              . del /:[ \t]*/ ": " . store Rx.space_in . eol ]

(* View: clause
   A colon-terminated clause header ("server:", "forward-zone:", ...) at column
   0, followed by its indented option lines, comments and blank lines. The
   recognised clause keywords are the section headers unbound.conf(5) defines. *)
let clause_kw = /server|remote-control|forward-zone|stub-zone|auth-zone|view|python|dynlib|dnstap|rpz|dnscrypt|cachedb|ipset/
let clause = [ key clause_kw . del /:[ \t]*/ ":" . eol
             . (entry | comment | empty)* ]

(* View: lns
   The Unbound lens: a sequence of clauses, top-level include directives,
   comments and blank lines. *)
let lns = (empty | comment | include | clause)*

(* View: filter *)
let filter = incl "/etc/unbound/unbound.conf"
           . incl "/etc/unbound/unbound.conf.d/*.conf"
           . Util.stdexcl

let xfm = transform lns filter


(* Test: Unbound.lns
   A realistic resolver config: a top-level include drop-in directive, a
   server clause with repeated interface:, access-control: and local-data:
   keys, a remote-control clause, and a forward-zone with two forward-addr:. *)
let conf = "# Unbound resolver configuration
include: \"/etc/unbound/unbound.conf.d/*.conf\"

server:
    verbosity: 1
    num-threads: 4
    interface: 127.0.0.1
    interface: ::1
    port: 53
    do-ip6: yes
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    local-zone: \"example.com.\" static
    local-data: \"www.example.com. IN A 192.0.2.1\"
    local-data: \"example.com. IN MX 10 mail.example.com.\"

remote-control:
    control-enable: yes
    control-interface: 127.0.0.1

forward-zone:
    name: \".\"
    forward-addr: 1.1.1.1
    forward-addr: 8.8.8.8@853
"

test Unbound.lns get conf =
  { "#comment" = "Unbound resolver configuration" }
  { "include" = "\"/etc/unbound/unbound.conf.d/*.conf\"" }
  {  }
  { "server"
    { "verbosity" = "1" }
    { "num-threads" = "4" }
    { "interface" = "127.0.0.1" }
    { "interface" = "::1" }
    { "port" = "53" }
    { "do-ip6" = "yes" }
    { "access-control" = "127.0.0.0/8 allow" }
    { "access-control" = "10.0.0.0/8 allow" }
    { "local-zone" = "\"example.com.\" static" }
    { "local-data" = "\"www.example.com. IN A 192.0.2.1\"" }
    { "local-data" = "\"example.com. IN MX 10 mail.example.com.\"" }
    {  } }
  { "remote-control"
    { "control-enable" = "yes" }
    { "control-interface" = "127.0.0.1" }
    {  } }
  { "forward-zone"
    { "name" = "\".\"" }
    { "forward-addr" = "1.1.1.1" }
    { "forward-addr" = "8.8.8.8@853" } }

(* Test: identity round-trip of an unmodified file *)
test Unbound.lns put conf after
    set "/server/verbosity" "1"
  = conf

(* Test: edit an existing value; every repeated key keeps its order and
   multiplicity around the change. *)
test Unbound.lns put conf after
    set "/server/verbosity" "2"
  = "# Unbound resolver configuration
include: \"/etc/unbound/unbound.conf.d/*.conf\"

server:
    verbosity: 2
    num-threads: 4
    interface: 127.0.0.1
    interface: ::1
    port: 53
    do-ip6: yes
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    local-zone: \"example.com.\" static
    local-data: \"www.example.com. IN A 192.0.2.1\"
    local-data: \"example.com. IN MX 10 mail.example.com.\"

remote-control:
    control-enable: yes
    control-interface: 127.0.0.1

forward-zone:
    name: \".\"
    forward-addr: 1.1.1.1
    forward-addr: 8.8.8.8@853
"

(* Test: edit a repeated key in place; the OTHER forward-addr and the sibling
   name: keep their order and count (the classic repeated-key fidelity trap). *)
test Unbound.lns put conf after
    set "/forward-zone/forward-addr[1]" "9.9.9.9"
  = "# Unbound resolver configuration
include: \"/etc/unbound/unbound.conf.d/*.conf\"

server:
    verbosity: 1
    num-threads: 4
    interface: 127.0.0.1
    interface: ::1
    port: 53
    do-ip6: yes
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    local-zone: \"example.com.\" static
    local-data: \"www.example.com. IN A 192.0.2.1\"
    local-data: \"example.com. IN MX 10 mail.example.com.\"

remote-control:
    control-enable: yes
    control-interface: 127.0.0.1

forward-zone:
    name: \".\"
    forward-addr: 9.9.9.9
    forward-addr: 8.8.8.8@853
"

(* Test: append a new repeated key at the end of the last clause; the two
   existing forward-addr: keep order + count and the new one lands after
   them. *)
test Unbound.lns put conf after
    set "/forward-zone/forward-addr[3]" "2.6.0.0"
  = "# Unbound resolver configuration
include: \"/etc/unbound/unbound.conf.d/*.conf\"

server:
    verbosity: 1
    num-threads: 4
    interface: 127.0.0.1
    interface: ::1
    port: 53
    do-ip6: yes
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    local-zone: \"example.com.\" static
    local-data: \"www.example.com. IN A 192.0.2.1\"
    local-data: \"example.com. IN MX 10 mail.example.com.\"

remote-control:
    control-enable: yes
    control-interface: 127.0.0.1

forward-zone:
    name: \".\"
    forward-addr: 1.1.1.1
    forward-addr: 8.8.8.8@853
    forward-addr: 2.6.0.0
"

(* Boundary (loud failure, never silent corruption): unlike a Caddyfile heredoc,
   unbound.conf has no context-free / back-reference construct, so any input the
   lens does not model FAILS `get` loudly instead of being silently misparsed.
   No API-layer rejection guard is therefore required. These tests pin that. *)

(* An indented option line with no enclosing clause header is rejected: option
   lines are only valid as clause children, never at the top level. *)
test Unbound.lns get "    verbosity: 1
" = *

(* A bare directive at column 0 that is neither a known clause header nor an
   include: is rejected (all real options live under a clause). *)
test Unbound.lns get "verbosity: 1
" = *
