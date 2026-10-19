#ifndef CANVAS_LOGIN_DARWIN_H
#define CANVAS_LOGIN_DARWIN_H

// Runs the login window on the main thread until canvas_login_stop is called
// or the user closes the window. Periodically reports cookies for `host`
// to the Go callback canvasCookies as "name\tvalue\n" lines.
void canvas_login_run(const char *url, const char *host, const char *title);

// Safe to call from any thread.
void canvas_login_stop(void);

#endif
