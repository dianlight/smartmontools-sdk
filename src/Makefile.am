## Process this file with automake to produce Makefile.in
#
# src/Makefile.am
#

# Suppress 'echo' if './configure --enable-silent-rules' or 'make V=0' is used.
ECHO_V_GEN = @$(AM_V_P) || echo '  GEN      $@'; ! $(AM_V_P) || echo

EXTRA_DIST = \
        cppcheck.sh \
        getversion.sh

CLEANFILES = cppcheck.txt shellcheck.txt

MAINTAINERCLEANFILES = Makefile.in

# Avoid automake warning: '.PHONY was already defined in condition ...'
phony = cppcheck htmlman pdfman shellcheck
.PHONY: $(phony)

if OS_WIN32_MINGW
phony += cleandist-win32 clean-vc distclean-vc maintainer-clean-vc
endif

# Create cppcheck report
cppcheck: cppcheck.txt

CPPCHECKFLAGS =
cppcheck.txt:
\t$(srcdir)/cppcheck.sh $(CPPCHECKFLAGS) > $@

# No man pages — htmlman and pdfman are no-ops
htmlman pdfman:
\t@:

# Create shellcheck report
shellcheck: shellcheck.txt

SHELLCHECK = shellcheck
# Exclude "SC3043 (warning): In POSIX sh, 'local' is undefined."
SHELLCHECKFLAGS = -S warning -e SC3043

SHELLSCRIPTS = \
        $(top_srcdir)/autogen.sh \
        $(srcdir)/cppcheck.sh \
        $(srcdir)/getversion.sh

shellcheck.txt: $(SHELLSCRIPTS)
\t$(ECHO_V_GEN) '  $(SHELLCHECK) $(SHELLCHECKFLAGS) $(SHELLSCRIPTS) > $@'
\t@$(SHELLCHECK) $(SHELLCHECKFLAGS) $(SHELLSCRIPTS) > $@ || \
\tif [ $$? = 1 ]; then echo "$@: issues found"; else rm -f $@; exit 1; fi

if OS_WIN32_MINGW
cleandist-win32 clean-vc distclean-vc maintainer-clean-vc:
\t@:
endif
