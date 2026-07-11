package interp

import (
	"fmt"
	"testing"
)

func diagOne(t *testing.T, module, binding, input string) {
	i := New(corpusSource())
	if _, err := i.LoadModule(module); err != nil {
		t.Fatalf("load %s: %v", module, err)
	}
	lens, err := i.LensValue(module, binding)
	if err != nil {
		t.Fatalf("lensvalue: %v", err)
	}
	size := len(input)
	regs, matched, err := lens.ctype.match(input, 0, size)
	fmt.Printf("\n==== %s.%s  len=%d matched=%v err=%v\n", module, binding, size, matched, err)
	if matched {
		fmt.Printf("top ctype matched [0:%d] of %d\n", regs[1], size)
		stop := regs[1]
		lo := stop - 40
		if lo < 0 {
			lo = 0
		}
		hi := stop + 60
		if hi > size {
			hi = size
		}
		fmt.Printf("STOPPED AT byte %d\n", stop)
		fmt.Printf("...before: %q\n", input[lo:stop])
		fmt.Printf("...after:  %q\n", input[stop:hi])
	}
}

func TestZtmpDiag(t *testing.T) {
	rsyslogConf := "# rsyslog v5 configuration file\n\n$ModLoad imuxsock # provides support for local system logging (e.g. via logger command)\n$ModLoad imklog   # provides kernel logging support (previously done by rklogd)\nmodule(load=\"immark\" markmessageperiod=\"60\" fakeoption=\"bar\") #provides --MARK-- message capability\n\ntimezone(id=\"CET\" offset=\"+01:00\")\n\n$UDPServerRun 514\n$InputTCPServerRun 514\n$ActionFileDefaultTemplate RSYSLOG_TraditionalFileFormat\n$ActionFileEnableSync on\n$IncludeConfig /etc/rsyslog.d/*.conf\n\n*.info;mail.none;authpriv.none;cron.none                /var/log/messages\nauthpriv.*                                              /var/log/secure\n*.emerg                                                 *\n*.*    @2.7.4.1\n*.*    @@2.7.4.1\n*.emerg :omusrmsg:*\n*.emerg :omusrmsg:foo,bar\n*.emerg | /dev/xconsole\n"
	diagOne(t, "Rsyslog", "lns", rsyslogConf)

	tmpl := "$template SpiceTmpl,\"%TIMESTAMP%.%TIMESTAMP:::date-subseconds% %syslogtag% %syslogseverity-text%:%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\\n\"\n"
	diagOne(t, "Rsyslog", "lns", tmpl)

	sipConf := "[general]\ncontext=default                 ; Default context for incoming calls\nudpbindaddr=0.0.0.0             ; IP address to bind UDP listen socket to (0.0.0.0 binds to all)\n; The address family of the bound UDP address is used to determine how Asterisk performs\n; DNS lookups. In cases a) and c) above, only A records are considered. In case b), only\n; AAAA records are considered. In case d), both A and AAAA records are considered. Note,\n\n\n[basic-options-title](!,superclass-template);a template for my preferred codecs !@#$%#@$%^^&%%^*&$%\n        #comment after the title\n        dtmfmode=rfc2833\n        context=from-office\n        type=friend\n\n\n[my-codecs](!)                    ; a template for my preferred codecs\n        disallow=all\n        allow=ilbc\n        allow=g729\n        allow=gsm\n        allow=g723\n        allow=ulaw\n\n[2133](natted-phone,my-codecs) ;;;;; some sort of comment\n       secret = peekaboo\n[2134](natted-phone,ulaw-phone)\n       secret = not_very_secret\n[2136](public-phone,ulaw-phone)\n       secret = not_very_secret_either\n"
	diagOne(t, "Sip_Conf", "lns", sipConf)
}
