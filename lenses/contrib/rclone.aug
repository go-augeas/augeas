(*
Module: Rclone
  Parses rclone remote-configuration files (rclone.conf).

Author: the go-augeas/augeas authors

About: Reference
  rclone(1) - https://rclone.org/docs/#config-config-file
  The file is an INI format: one [remote] section per configured remote,
  each holding "key = value" options. Values are stored verbatim to the
  end of the line so OAuth token blobs (JSON containing '"', ':' and '{}')
  round-trip faithfully.

About: License
  This file is licensed under the LGPL v2+, like the rest of Augeas.

About: Lens Usage
  Sample usage of this lens in augtool

    > print /files/root/.config/rclone/rclone.conf

About: Configuration files
  This lens applies to rclone.conf wherever it lives.

About: Examples
  The tests at the end of this file document the supported syntax.
*)

module Rclone =

autoload xfm

(* View: comment
     A "#" or ";" comment line *)
let comment = IniFile.comment IniFile.comment_re "#"

(* View: empty *)
let empty = Util.empty

(* View: eol *)
let eol = Util.eol

(* View: sep
     Key/value separator, normalised to " = " *)
let sep = Sep.space_equal

(* View: entry
   A "key = value" line; value stored verbatim to end of line. *)
let entry_kw = /[A-Za-z_][A-Za-z0-9_]*/
let entry = [ Util.indent . key entry_kw . sep . store Rx.space_in . eol ]

(* View: title
   A [remote] section. Remote names may hold most printable characters
   except ']' and '/'. *)
let title = IniFile.title IniFile.record_re

(* View: record *)
let record = IniFile.record title (entry|comment)

(* View: lns
   The Rclone lens *)
let lns = IniFile.lns record comment

(* View: filter *)
let filter = incl "/root/.config/rclone/rclone.conf"
           . incl "/home/*/.config/rclone/rclone.conf"
           . incl "/etc/rclone.conf"
           . Util.stdexcl

let xfm = transform lns filter

(* Test: Rclone.lns
   Two remotes: an S3 bucket and a Google Drive with an OAuth token blob *)
let conf = "[minio]
type = s3
provider = Minio
access_key_id = AKIAIOSFODNN7EXAMPLE
secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
endpoint = https://minio.example.com:9000
region = us-east-1

[gdrive]
type = drive
scope = drive
token = {\"access_token\":\"ya29.EXAMPLE\",\"token_type\":\"Bearer\",\"expiry\":\"2026-01-01T00:00:00Z\"}
"

test Rclone.lns get conf =
  { "minio"
    { "type" = "s3" }
    { "provider" = "Minio" }
    { "access_key_id" = "AKIAIOSFODNN7EXAMPLE" }
    { "secret_access_key" = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" }
    { "endpoint" = "https://minio.example.com:9000" }
    { "region" = "us-east-1" }
    {  } }
  { "gdrive"
    { "type" = "drive" }
    { "scope" = "drive" }
    { "token" = "{\"access_token\":\"ya29.EXAMPLE\",\"token_type\":\"Bearer\",\"expiry\":\"2026-01-01T00:00:00Z\"}" } }

(* Test: identity round-trip *)
test Rclone.lns put conf after
    set "/minio/region" "us-east-1"
  = conf

(* Test: mutate two existing values and write them back *)
test Rclone.lns put conf after
    set "/minio/region" "eu-west-1" ;
    set "/minio/provider" "Ceph"
  = "[minio]
type = s3
provider = Ceph
access_key_id = AKIAIOSFODNN7EXAMPLE
secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
endpoint = https://minio.example.com:9000
region = eu-west-1

[gdrive]
type = drive
scope = drive
token = {\"access_token\":\"ya29.EXAMPLE\",\"token_type\":\"Bearer\",\"expiry\":\"2026-01-01T00:00:00Z\"}
"
