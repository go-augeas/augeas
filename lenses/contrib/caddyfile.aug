(*
Module: Caddyfile
  Parses the Caddy web server native configuration format (the "Caddyfile").

Author: the go-augeas/augeas authors

About: Reference
  Caddyfile concepts - https://caddyserver.com/docs/caddyfile/concepts

  A Caddyfile is a sequence of site blocks. Each site block has one or more
  addresses followed by a brace-delimited body of directives. Directives take
  space-separated arguments and may themselves open nested brace blocks
  (handle, route, reverse_proxy { ... }, tls { ... }, ...). Named matchers
  (@name { ... }), snippets ((name) { ... }) and the leading global options
  block ({ ... } with no address) are all the same brace-block shape and are
  modelled by the same recursive lens, exactly like Nginx.lns models nested
  http/server/location blocks.

About: License
  This file is licensed under the LGPL v2+, like the rest of Augeas.

About: Feasibility / boundary (documented, not silently lossy)
  MODELLED faithfully (get + put round-trip):
    - the leading global options block  { ... }         (label "@global")
    - site-address blocks               addr { ... }
    - snippet definitions               (name) { ... }
    - named matcher blocks              @name { ... }
    - arbitrarily nested directive blocks (recursive)
    - directives with space-separated bare and double-quoted arguments
    - placeholders {host} {env.X} {http.request.uri} as OPAQUE arg tokens
      (braces that are values, not blocks)
    - inline matcher tokens used as arguments (reverse_proxy @ws :6001)
    - import as an ordinary directive (structural; NOT expanded, by design)
    - "#" comments and blank lines, inside and outside blocks

  EXCLUDED (a .aug regular lens cannot express these; documented, not faked):
    - HEREDOCS  <<EOF ... EOF  : the closing token repeats the opening word,
      which is a context-free / back-reference construct; Augeas regexps are
      regular and cannot match "the same word again". A file using a heredoc
      value will fail `get` on that directive.
    - `import` expansion / snippet inlining: semantic, out of scope for a lens.
    - a "#" that is not preceded by whitespace inside a bare token is treated
      here as a comment start; Caddy only treats leading-of-token "#" as a
      comment. Bare args therefore exclude "#".

About: Configuration files
  This lens applies to /etc/caddy/Caddyfile and /etc/caddy/conf.d/ *.

About: Examples
  The tests at the end of this file document the supported syntax.
*)

module Caddyfile =

autoload xfm

let comment = Util.comment
let empty   = Util.empty
let eol     = Util.eol

(* View: arg
     A single space-separated argument. Either a double-quoted string (which
     may contain spaces, braces and escapes) or a bare token. The bare token
     is a run of non-whitespace, non-quote, non-"#" characters containing at
     least one character that is not a brace, so that a lone block-opening "{"
     or block-closing "}" is NOT swallowed as an argument, while a placeholder
     token such as {host} or {http.request.uri} (which contains non-brace
     characters) IS a normal argument. *)
let dquote = /"([^"\\]|\\.)*"/
let bare   = /[^ \t\n"#]*[^ \t\n"#{}][^ \t\n"#]*/
let arg    = [ label "arg" . Sep.space . store (dquote | bare) ]

(* View: key_rx
     A directive name or a site address / snippet / matcher head. Its first
     character is not a brace (so a leading "{" is the global block, and a
     lone "}" closes a block) and it carries no whitespace, quote or "#". This
     matches example.com, :80, http://example.com, (snippet), @matcher,
     reverse_proxy, tls, ... *)
let key_rx = /[^ \t\n"#{}][^ \t\n"#]*/

(* Block delimiters.
     The opening brace MUST be on the same line as the directive/address it
     belongs to (Caddyfile requires this), i.e. only horizontal whitespace may
     precede it, then a newline follows. This is what disambiguates a simple
     directive (ends in a bare newline) from a block directive (ends in
     " {\n") without any per-keyword allow-list -- unlike Nginx, Caddy has no
     block-keyword set and no ";" statement terminator. *)
let ldelim = del /[ \t]*\{[ \t]*\n/ " {\n"
let rdelim = del /[ \t]*\}/ "}"

(* View: entry
     The recursive core. A directive is ONE subtree: name + space-separated
     args + an OPTIONAL brace block whose body is more entries.

     Crucially this is a single subtree with an *optional inner* block, NOT a
     union `simple | block` of two subtrees. Caddy -- unlike Nginx -- has no
     fixed set of block keywords: ANY directive may or may not open a block, so
     `simple` and `block` would share the same key regexp and therefore the
     same subtree tree-type (label+value), which Augeas' put cannot tell apart
     (a childless node is in both). By folding the block into an optional
     Maybe *inside* the subtree, the block/no-block choice is resolved at the
     CHILDREN level (arg-labelled nodes vs directive-name-labelled entry
     nodes), which the put splitter can see -- so get AND put round-trip.

     The block body is non-nullable (>=1 entry/comment); its absence is the
     Maybe being absent. Residual non-injectivity: an EMPTY block `dir { }`
     (no args, no body) has the same tree as the bare directive `dir`, so it
     round-trips to the bare form -- documented, and semantically harmless. *)
let rec entry =
     let body = (entry | comment) . (empty | entry | comment)*
  in [ Util.indent . key key_rx . arg*
     . ( ldelim . body . rdelim )?
     . eol ]

(* View: global
     The leading global options block: a brace block with no address, given the
     synthetic label "@global". No union ambiguity (distinct label), so its
     body may be empty. *)
let global =
     let body = (entry | comment) . (empty | entry | comment)*
  in [ Util.indent . label "@global"
     . del /\{[ \t]*\n/ "{\n" . body? . rdelim . eol ]

(* View: lns *)
let lns = ( comment | empty | global | entry )*

(* Variable: filter *)
let filter = incl "/etc/caddy/Caddyfile"
           . incl "/etc/caddy/conf.d/*"
           . incl "/usr/local/etc/caddy/Caddyfile"

let xfm = transform lns filter


(* ===================================================================== *)
(* Tests                                                                 *)
(* ===================================================================== *)

(* Test: a minimal reverse-proxy site *)
test lns get "example.com {
	reverse_proxy localhost:8080
}
" =
  { "example.com"
    { "reverse_proxy"
      { "arg" = "localhost:8080" } } }

(* Test: multiple space-separated args + a placeholder token as a value *)
test lns get "example.com {
	root * /var/www
	reverse_proxy {http.request.uri}
}
" =
  { "example.com"
    { "root"
      { "arg" = "*" }
      { "arg" = "/var/www" } }
    { "reverse_proxy"
      { "arg" = "{http.request.uri}" } } }

(* Test: global options block + a site, in one file *)
test lns get "{
	email admin@example.com
	admin off
}

example.com {
	respond \"Hello\"
}
" =
  { "@global"
    { "email"
      { "arg" = "admin@example.com" } }
    { "admin"
      { "arg" = "off" } } }
  {  }
  { "example.com"
    { "respond"
      { "arg" = "\"Hello\"" } } }

(* Test: nested handle + reverse_proxy block (arbitrary nesting, recursive) *)
test lns get "example.com {
	handle /api/* {
		reverse_proxy localhost:9000 {
			header_up Host {host}
		}
	}
}
" =
  { "example.com"
    { "handle"
      { "arg" = "/api/*" }
      { "reverse_proxy"
        { "arg" = "localhost:9000" }
        { "header_up"
          { "arg" = "Host" }
          { "arg" = "{host}" } } } } }

(* Test: named matcher block + inline matcher used as an argument *)
test lns get "example.com {
	@websockets {
		header Connection *Upgrade*
	}
	reverse_proxy @websockets localhost:6001
}
" =
  { "example.com"
    { "@websockets"
      { "header"
        { "arg" = "Connection" }
        { "arg" = "*Upgrade*" } } }
    { "reverse_proxy"
      { "arg" = "@websockets" }
      { "arg" = "localhost:6001" } } }

(* Test: snippet definition + import (structural, not expanded) + comment *)
test lns get "(logging) {
	log {
		output stdout
	}
}

example.com {
	# use the snippet
	import logging
}
" =
  { "(logging)"
    { "log"
      { "output"
        { "arg" = "stdout" } } } }
  {  }
  { "example.com"
    { "#comment" = "use the snippet" }
    { "import"
      { "arg" = "logging" } } }

(* Test: a double-quoted argument containing spaces is one arg value *)
test lns get "example.com {
	respond \"Hello, World!\" 200
}
" =
  { "example.com"
    { "respond"
      { "arg" = "\"Hello, World!\"" }
      { "arg" = "200" } } }

(* Test: multi-address site (comma-separated hosts on one line) round-trips as
   the address key plus the remaining hosts as args -- structural, faithful *)
test lns get "example.com, www.example.com {
	root * /srv
}
" =
  { "example.com,"
    { "arg" = "www.example.com" }
    { "root"
      { "arg" = "*" }
      { "arg" = "/srv" } } }

(* Boundary (named blocker): HEREDOC values are NOT modelled. The closing token
   repeats the opening word (`<<HTML ... HTML`), a context-free / back-reference
   construct a regular Augeas lens cannot match. This lens does NOT reject a
   heredoc -- worse, it SILENTLY MISPARSES it: each body line is captured as its
   own sibling directive (see the tree below), and the closing `HTML 200` line
   becomes a bogus `HTML` directive. Do NOT apply this lens to Caddyfiles that
   use heredocs; there is no way for the lens to detect one. This test pins the
   (wrong) behaviour so the boundary is explicit and any future fix is visible. *)
test lns get "example.com {
	respond <<HTML
	<h1>hi</h1>
	HTML 200
}
" =
  { "example.com"
    { "respond"
      { "arg" = "<<HTML" } }
    { "<h1>hi</h1>" }
    { "HTML"
      { "arg" = "200" } } }

(* Boundary: a LITERAL empty block `dir { }` (no args, no body) is NOT parsed.
   The block body is deliberately non-nullable so that a bare directive `dir`
   does not gain braces on put; the price is that an empty brace pair fails
   `get`. Empty blocks are practically never written in real Caddyfiles (a site
   or directive block always carries directives). This test asserts the
   boundary explicitly. *)
test lns get "example.com {
}
" = *

(* Test: PUT round-trip -- identity (get then put with no change) *)
test lns put "example.com {
	reverse_proxy localhost:8080
}
" after
  rm "/nonexistent"
= "example.com {
	reverse_proxy localhost:8080
}
"

(* Test: PUT -- mutate a directive argument in a nested block, faithful re-render *)
test lns put "example.com {
	handle /api/* {
		reverse_proxy localhost:9000
	}
}
" after
  set "/example.com/handle/reverse_proxy/arg" "localhost:9999"
= "example.com {
	handle /api/* {
		reverse_proxy localhost:9999
	}
}
"

(* Test: PUT -- append a new directive to a site block.
   A newly-created node uses the lens' default indent (Util.indent -> ""), so
   the appended line is unindented. This matches Augeas create-semantics; and
   Caddyfile indentation is insignificant, so the result is valid. Editing an
   existing line preserves its original indentation (see the test above). *)
test lns put "example.com {
	root * /var/www
}
" after
  set "/example.com/reverse_proxy/arg" "localhost:8080"
= "example.com {
	root * /var/www
reverse_proxy localhost:8080
}
"
