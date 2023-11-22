/*
 * Copyright 2023 Google LLC

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 *      http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
*/

// Source: https://raw.githubusercontent.com/google/oss-fuzz/master/infra/base-images/base-builder-go/gosigfuzz.c

#include<stdlib.h>
#include<signal.h>

static void fixSignalHandler(int signum) {
    struct sigaction new_action;
    struct sigaction old_action;
    sigemptyset (&new_action.sa_mask);
    sigaction (signum, NULL, &old_action);
    new_action.sa_flags = old_action.sa_flags | SA_ONSTACK;
    new_action.sa_sigaction = old_action.sa_sigaction;
    new_action.sa_handler = old_action.sa_handler;
    sigaction (signum, &new_action, NULL);
}

static void FixStackSignalHandler() {
    fixSignalHandler(SIGSEGV);
    fixSignalHandler(SIGABRT);
    fixSignalHandler(SIGALRM);
    fixSignalHandler(SIGINT);
    fixSignalHandler(SIGTERM);
    fixSignalHandler(SIGBUS);
    fixSignalHandler(SIGFPE);
    fixSignalHandler(SIGXFSZ);
    fixSignalHandler(SIGUSR1);
    fixSignalHandler(SIGUSR2);
}

int LLVMFuzzerInitialize(int *argc, char ***argv) {
    FixStackSignalHandler();
    return 0;
}
