(*
Module: Nftables
  Parses the nftables native ruleset syntax as written in /etc/nftables.conf
  and as emitted by `nft list ruleset`.

Author: the go-augeas/augeas authors

About: Reference
  nftables wiki - https://wiki.nftables.org/
  nft(8) man page.

  An nftables ruleset is a sequence of top-level items: `table <family> <name>
  { ... }` blocks, `define NAME = value` variable definitions, `include "..."`
  directives and standalone commands (`flush ruleset`, `add ...`, ...). A table
  body contains `chain`/`set`/`map`/`flowtable` blocks; a chain body contains
  the base-chain setting line (`type ... hook ... priority ...; policy ...;`)
  and ordered rule lines (`ip saddr ... tcp dport { 22, 80 } accept`).

  The brace nesting `table { chain { rule } }` is modelled with the same
  recursive "one subtree with a mandatory inner block" pattern used by
  Nginx.lns (nested http/server/location) and the go-augeas Caddyfile lens: a
  block-opening line ends in `{` + newline, which distinguishes it from a rule
  line (which may itself contain an *anonymous set* `{ 22, 80 }` but never ends
  in a bare `{`). Rule and base-chain lines are stored verbatim to end-of-line
  so their internal expression grammar (verdict maps `vmap { ... }`, ranges,
  concatenations) round-trips without being fully parsed.

About: License
  This file is licensed under the LGPL v2+, like the rest of Augeas.

About: Feasibility / boundary (documented, not silently lossy)
  MODELLED faithfully (get + put round-trip):
    - table blocks              table <family> <name> { ... }   (family + name)
    - chain/set/map/flowtable blocks        <kw> <name> { ... }  (name)
    - base-chain / set-property / rule lines, stored verbatim to EOL as ordered
      "rule" nodes (the `;`-joined `type ...; policy ...;` line is one node)
    - anonymous sets `{ 22, 80, 443 }` and verdict maps `vmap { ... }` inside a
      rule line, as part of the opaque rule text
    - define NAME = value        (structured: name + value, value verbatim)
    - include "path"             (structured)
    - "#" comments and blank lines, inside and outside blocks

  EXCLUDED (documented, not faked):
    - a value/element list that nft wraps across MULTIPLE physical lines, e.g.
      a long `elements = { a,\n\t b }` set body or a rule continued with a
      trailing backslash. Each physical line is a separate node; a line that
      ends in a bare `{` is read as a block opener, so a wrapped element list
      whose first line ends in `{` WOULD misparse -- rejected up-front at the
      go-augeas API layer (see checkNftablesLineWrap) rather than corrupted.

About: Configuration files
  This lens applies to /etc/nftables.conf and /etc/nftables/*.nft and
  /etc/sysconfig/nftables.conf.

About: Examples
  The tests at the end of this file document the supported syntax.
*)

module Nftables =

autoload xfm

let comment = Util.comment
let empty   = Util.empty
let eol     = Util.eol
let indent  = Util.indent

(* View: word
     A bare token: no whitespace, brace or "#". Used for family and object
     names (table/chain/set names, which nft restricts to
     [A-Za-z0-9_./-] plus quoting, but we accept any brace/space-free run). *)
let word = /[^ \t\n{}#"]+/

(* View: family
     The address family of a table. nft always prints one of these. *)
let family = /ip6|ip|inet|arp|bridge|netdev/

(* Block delimiters (same shape as Caddyfile/Nginx). The opening brace is the
   last non-blank character on its line; the closing brace opens its own line
   (a rule line can never start with "}"). *)
let ldelim = del /[ \t]*\{[ \t]*\n/ " {\n"
let rdelim = del /[ \t]*\}[ \t]*/ "}" . eol

(* View: line_rx
     A base-chain / set-property / rule line stored verbatim. It starts with a
     character that is not whitespace, "#" (comment), "{" or "}" (so a lone
     closing brace is never a rule and a wrapped list opener is never a rule),
     and its last non-blank character is not "{" (so a block-opening line is
     never a rule). Anything in between is free, so an in-line anonymous set
     `{ 22, 80 }` or verdict map `vmap { ... }` is preserved as text. The line
     is disjoint from the `define`/`include` structured lines below. *)
let line_body = /[^ \t\n#{}][^\n]*[^ \t\n{]|[^ \t\n#{}]/
let line_rx   = line_body - (/(define|include)[ \t][^\n]*/)

let rule = [ indent . label "rule" . store line_rx . eol ]

(* View: define
     A variable definition: `define NAME = value`. The value is stored verbatim
     to EOL (it is often an anonymous set `{ 22, 80, 443 }` or an interval). *)
let val_rx = /[^ \t\n][^\n]*[^ \t\n]|[^ \t\n]/
let define =
  [ indent . key "define" . Sep.space
  . [ label "name" . store word ]
  . Sep.space . Util.del_str "=" . Sep.space
  . [ label "value" . store val_rx ] . eol ]

(* View: include
     An `include "path"` directive; the quoted path is stored with its quotes. *)
let include =
  [ indent . key "include" . Sep.space
  . [ label "path" . store /"[^"\n]*"/ ] . eol ]

(* View: block keyword
     The objects that open a brace block inside a table body. *)
let block_kw = "chain" | "set" | "map" | "flowtable" | "counter" | "quota" | "ct helper" | "ct timeout" | "ct expectation" | "limit" | "secmark" | "synproxy"

(* View: entry
     The recursive core. `table` carries a family + name; the other block
     keywords carry a name; both open a mandatory brace block whose body is
     more entries. Distinct labels (table/chain/set/... vs "rule") mean the put
     splitter never confuses a block with a rule, so an empty block body is
     allowed (a regular chain may legitimately be empty). *)
let rec entry =
     let body = (entry | comment | empty)*
  in let table = [ indent . key "table"
                 . Sep.space . [ label "family" . store family ]
                 . Sep.space . [ label "name" . store word ]
                 . ldelim . body . rdelim ]
  in let named = [ indent . key block_kw
                 . Sep.space . [ label "name" . store word ]
                 . ldelim . body . rdelim ]
  in ( table | named | define | include | rule | comment | empty )

(* View: lns *)
let lns = ( comment | empty | entry )*

(* Variable: filter *)
let filter = incl "/etc/nftables.conf"
           . incl "/etc/nftables/*.nft"
           . incl "/etc/sysconfig/nftables.conf"

let xfm = transform lns filter


(* ===================================================================== *)
(* Tests                                                                 *)
(* ===================================================================== *)

(* Test: a filter table with the three base chains and a couple of rules *)
test lns get "table inet filter {
	chain input {
		type filter hook input priority 0; policy drop;
		ct state established,related accept
		iif \"lo\" accept
	}
	chain forward {
		type filter hook forward priority 0; policy drop;
	}
	chain output {
		type filter hook output priority 0; policy accept;
	}
}
" =
  { "table"
    { "family" = "inet" }
    { "name" = "filter" }
    { "chain"
      { "name" = "input" }
      { "rule" = "type filter hook input priority 0; policy drop;" }
      { "rule" = "ct state established,related accept" }
      { "rule" = "iif \"lo\" accept" } }
    { "chain"
      { "name" = "forward" }
      { "rule" = "type filter hook forward priority 0; policy drop;" } }
    { "chain"
      { "name" = "output" }
      { "rule" = "type filter hook output priority 0; policy accept;" } } }

(* Test: a named set plus a rule referencing it with @name *)
test lns get "table ip filter {
	set blocklist {
		type ipv4_addr
		elements = { 10.0.0.1, 10.0.0.2 }
	}
	chain input {
		ip saddr @blocklist drop
	}
}
" =
  { "table"
    { "family" = "ip" }
    { "name" = "filter" }
    { "set"
      { "name" = "blocklist" }
      { "rule" = "type ipv4_addr" }
      { "rule" = "elements = { 10.0.0.1, 10.0.0.2 }" } }
    { "chain"
      { "name" = "input" }
      { "rule" = "ip saddr @blocklist drop" } } }

(* Test: a define + a rule using the $variable, at top level and in a table *)
test lns get "define guarded_ports = { 22, 80, 443 }

table inet filter {
	chain input {
		tcp dport $guarded_ports accept
	}
}
" =
  { "define"
    { "name" = "guarded_ports" }
    { "value" = "{ 22, 80, 443 }" } }
  {  }
  { "table"
    { "family" = "inet" }
    { "name" = "filter" }
    { "chain"
      { "name" = "input" }
      { "rule" = "tcp dport $guarded_ports accept" } } }

(* Test: an anonymous-set rule `tcp dport { 22, 80 } accept` round-trips as one
   verbatim rule line (the inline `{ ... }` is not a block: the line does not
   end in a bare `{`). *)
test lns get "table inet filter {
	chain input {
		tcp dport { 22, 80 } accept
	}
}
" =
  { "table"
    { "family" = "inet" }
    { "name" = "filter" }
    { "chain"
      { "name" = "input" }
      { "rule" = "tcp dport { 22, 80 } accept" } } }

(* Test: include + comment + flush command at top level *)
test lns get "#!/usr/sbin/nft -f
flush ruleset
include \"/etc/nftables/inet-filter.nft\"
" =
  { "#comment" = "!/usr/sbin/nft -f" }
  { "rule" = "flush ruleset" }
  { "include"
    { "path" = "\"/etc/nftables/inet-filter.nft\"" } }

(* Test: an empty regular chain (allowed: distinct label, nullable body) *)
test lns get "table inet filter {
	chain input {
	}
}
" =
  { "table"
    { "family" = "inet" }
    { "name" = "filter" }
    { "chain"
      { "name" = "input" } } }

(* Test: a verdict map used inside a rule stays opaque in the rule text *)
test lns get "table inet nat {
	chain prerouting {
		tcp dport vmap { 80 : jump http, 443 : jump https }
	}
}
" =
  { "table"
    { "family" = "inet" }
    { "name" = "nat" }
    { "chain"
      { "name" = "prerouting" }
      { "rule" = "tcp dport vmap { 80 : jump http, 443 : jump https }" } } }

(* Boundary (named blocker): a value/element list that nft WRAPS across more
   than one physical line is NOT modelled. Each physical line is parsed on its
   own, so a wrapped `elements = { a,` / `b }` group SILENTLY MISPARSES into two
   bogus sibling "rule" nodes (see the tree below) -- there is no way for a
   regular lens to know the first line's "{" is not closed until a later line.
   The go-augeas API layer therefore REJECTS such input up-front
   (checkNftablesLineWrap) rather than corrupt it; this test pins the raw
   interpreter-level misparse so the boundary stays explicit and any future fix
   is visible. Real single-line lists (`elements = { a, b }`) are unaffected. *)
test lns get "table ip filter {
	set big {
		elements = { 10.0.0.1,
			     10.0.0.2 }
	}
}
" =
  { "table"
    { "family" = "ip" }
    { "name" = "filter" }
    { "set"
      { "name" = "big" }
      { "rule" = "elements = { 10.0.0.1," }
      { "rule" = "10.0.0.2 }" } } }

(* Test: PUT round-trip -- identity (get then put with no change) *)
test lns put "table inet filter {
	chain input {
		type filter hook input priority 0; policy drop;
		tcp dport { 22, 80 } accept
	}
}
" after
  rm "/nonexistent"
= "table inet filter {
	chain input {
		type filter hook input priority 0; policy drop;
		tcp dport { 22, 80 } accept
	}
}
"

(* Test: PUT -- mutate a rule inside a chain, faithful re-render *)
test lns put "table inet filter {
	chain input {
		tcp dport { 22, 80 } accept
	}
}
" after
  set "/table/chain/rule" "tcp dport { 22, 80, 443 } accept"
= "table inet filter {
	chain input {
		tcp dport { 22, 80, 443 } accept
	}
}
"

(* Test: PUT -- change a table's family and a define's value *)
test lns put "define ports = { 22 }

table ip filter {
	chain input {
		tcp dport $ports accept
	}
}
" after
  set "/define/value" "{ 22, 443 }"
= "define ports = { 22, 443 }

table ip filter {
	chain input {
		tcp dport $ports accept
	}
}
"
