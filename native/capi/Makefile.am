## Process this file with automake to produce Makefile.in
#
# capi/Makefile.am
#
# The C ABI wrapper (smartmon_c_api.cpp/.h) is intentionally NOT built here.
# It is a shared library cross-compiled for 7 release targets (incl. darwin
# and musl via osxcross/zig) by scripts/build-wrapper.sh, which hand-drives
# CXX/CXXFLAGS per target far more directly than coaxing libtool into
# emitting a non-PIC static archive and a PIC shared object from the same
# sources would allow. This Makefile.am only ships the sources in `make
# dist` and installs the public header for out-of-tree consumers.

MAINTAINERCLEANFILES = Makefile.in

EXTRA_DIST = \
        smartmon_c_api.cpp \
        smartmon_c_api.h

if INSTALL_DEVEL_INC
incsmartmondir = $(includedir)/smartmon
incsmartmon_DATA = smartmon_c_api.h
endif
