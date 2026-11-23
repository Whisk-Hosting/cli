// Whisk push notifications in a service worker. Served by Whisk on every app's address at
// /.whisk/push-sw.js, which /.whisk/push.js registers at the scope /.whisk/push/; an app with its
// own service worker imports it instead: importScripts("/.whisk/push-sw.js"). MIT, part of
// Whisk's public contract.
(function () {
  "use strict";
  if (self.__whiskPush) return;
  self.__whiskPush = true;
  var own = self.registration && /\/\.whisk\/push\/$/.test(self.registration.scope);

  if (own) {
    self.addEventListener("install", function () { self.skipWaiting(); });
    self.addEventListener("activate", function (event) { event.waitUntil(self.clients.claim()); });
  }

  self.addEventListener("push", function (event) {
    var n = {};
    try {
      n = event.data ? event.data.json() : {};
    } catch (e) {
      n = { title: event.data ? event.data.text() : "" };
    }
    // A Whisk notification always carries its send's id; leave any other push to the app's own
    // handler.
    if (!n || !n.send) return;
    var options = {
      body: n.body || "",
      tag: n.tag || undefined,
      renotify: !!n.tag,
      icon: n.icon || undefined,
      data: { url: n.url || "/", send: n.send, data: n.data || null },
    };
    event.waitUntil(self.registration.showNotification(n.title || "", options));
  });

  self.addEventListener("notificationclick", function (event) {
    var d = event.notification.data || {};
    if (!d.send) return;
    event.notification.close();
    var url = new URL(d.url || "/", self.location.origin).href;
    event.waitUntil(self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (list) {
      for (var i = 0; i < list.length; i++) {
        if (list[i].url === url && "focus" in list[i]) return list[i].focus();
      }
      for (var j = 0; j < list.length; j++) {
        var c = list[j];
        if (new URL(c.url).origin === self.location.origin && "navigate" in c) {
          return c.navigate(url).then(function (w) { return w ? w.focus() : self.clients.openWindow(url); });
        }
      }
      return self.clients.openWindow(url);
    }));
  });

  // A browser that renews a subscription asks for a new one under the same key; the next page
  // load's Whisk.push.sync hands it to the app.
  self.addEventListener("pushsubscriptionchange", function (event) {
    var old = event.oldSubscription;
    var key = old && old.options && old.options.applicationServerKey;
    if (!key) return;
    event.waitUntil(self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key }));
  });
})();
