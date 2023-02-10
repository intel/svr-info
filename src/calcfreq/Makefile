#!make
#
# Copyright (C) 2023 Intel Corporation
# SPDX-License-Identifier: MIT
#

VERSION ?= dev

calcfreq : calcfreq.c
	gcc -D VERSION=\"$(VERSION)\" calcfreq.c -lpthread -o calcfreq -static

all : calcfreq

clean :
	rm -f calcfreq