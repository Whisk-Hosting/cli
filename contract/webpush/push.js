// Whisk push notifications for a page. Served by Whisk on every app's address at
// /.whisk/push.js; MIT, part of Whisk's public contract.
//
//   await Whisk.push.state()      "unsupported" | "needs-install" | "denied" | "off" | "on"
//   await Whisk.push.enable({ register })   from a tap: asks, subscribes, hands the device to
//                                           register(subscription, { label })
//   await Whisk.push.sync({ register })     on each page load: hands the device over again
//   await Whisk.push.disable({ unregister })
//
// register and unregister are the page's own calls to its server, which keeps the device with
// POST /v1/orgs/<org>/apps/<app>/push/devices and its service token. Pass { registration } to
// use the page's own service worker instead, which then imports /.whisk/push-sw.js.
(function () {
  "use strict";
  var SW = "/.whisk/push-sw.js";
  var SCOPE = "/.whisk/push/";

  function supported() {
    return "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
  }

  function apple() {
    var ua = navigator.userAgent || "";
    return /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1);
  }

  function installed() {
    return navigator.standalone === true ||
      (window.matchMedia && window.matchMedia("(display-mode: standalone)").matches);
  }

  function fail(code, message) {
    var e = new Error(message);
    e.code = code;
    return e;
  }

  function bytes(b64) {
    var s = b64.replace(/-/g, "+").replace(/_/g, "/");
    s += "===".slice((s.length + 3) % 4);
    var raw = atob(s), out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  function same(a, b) {
    if (!a || !b) return false;
    a = new Uint8Array(a);
    if (a.length !== b.length) return false;
    for (var i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
    return true;
  }

  var keyOnce = null;
  function key() {
    if (!keyOnce) {
      keyOnce = fetch("/.whisk/push/key", { credentials: "omit" }).then(function (r) {
        if (!r.ok) throw fail("PUSH_UNAVAILABLE", "Whisk did not give this app's push key.");
        return r.json();
      }).then(function (j) { return bytes(j.key); });
      keyOnce.catch(function () { keyOnce = null; });
    }
    return keyOnce;
  }

  function registration(opts) {
    if (opts && opts.registration) return Promise.resolve(opts.registration);
    return navigator.serviceWorker.register(SW, { scope: SCOPE }).then(function (reg) {
      if (reg.active) return reg;
      return new Promise(function (resolve) {
        var w = reg.installing || reg.waiting;
        if (!w) return resolve(reg);
        w.addEventListener("statechange", function () {
          if (w.state === "activated") resolve(reg);
        });
      });
    });
  }

  function existing(opts) {
    if (opts && opts.registration) return opts.registration.pushManager.getSubscription();
    return navigator.serviceWorker.getRegistration(SCOPE).then(function (reg) {
      return reg ? reg.pushManager.getSubscription() : null;
    });
  }

  function label() {
    var ua = navigator.userAgent || "";
    var b = /Edg\//.test(ua) ? "Edge" : /SamsungBrowser/.test(ua) ? "Samsung Internet" :
      /Firefox\/|FxiOS/.test(ua) ? "Firefox" : /Chrome\/|CriOS/.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "";
    var s = /iPhone/.test(ua) ? "iPhone" : /iPad/.test(ua) ? "iPad" : /Android/.test(ua) ? "Android" :
      /Mac OS X/.test(ua) ? "Mac" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
    return b && s ? b + " on " + s : b || s;
  }

  function hand(opts, sub) {
    var json = sub.toJSON();
    if (opts && typeof opts.register === "function") {
      return Promise.resolve(opts.register(json, { label: label() })).then(function () { return json; });
    }
    throw fail("PUSH_NO_REGISTER", "Pass register: a function that sends the subscription to your server.");
  }

  function state(opts) {
    if (!supported()) {
      return Promise.resolve(apple() && !installed() ? "needs-install" : "unsupported");
    }
    if (Notification.permission === "denied") return Promise.resolve("denied");
    if (Notification.permission !== "granted") return Promise.resolve("off");
    return Promise.all([existing(opts), key()]).then(function (got) {
      var sub = got[0];
      return sub && same(sub.options && sub.options.applicationServerKey, got[1]) ? "on" : "off";
    }, function () { return "off"; });
  }

  // enable must run from a tap or click: browsers only ask for permission then.
  function enable(opts) {
    if (!supported()) {
      return Promise.reject(apple() && !installed()
        ? fail("PUSH_NEEDS_INSTALL", "On iPhone and iPad, add this app to the Home Screen first, then turn notifications on there.")
        : fail("PUSH_UNSUPPORTED", "This browser cannot show notifications."));
    }
    var asked = Notification.permission === "granted" ? Promise.resolve("granted") : Notification.requestPermission();
    return Promise.resolve(asked).then(function (p) {
      if (p !== "granted") {
        throw fail("PUSH_DENIED", p === "denied"
          ? "Notifications are blocked for this site. Allow them in the browser's site settings."
          : "Notifications were not allowed.");
      }
      return Promise.all([registration(opts), key()]);
    }).then(function (got) {
      var reg = got[0], k = got[1];
      return reg.pushManager.getSubscription().then(function (sub) {
        if (sub && same(sub.options && sub.options.applicationServerKey, k)) return sub;
        var gone = sub ? sub.unsubscribe() : Promise.resolve();
        return gone.then(function () {
          return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: k });
        });
      });
    }).then(function (sub) { return hand(opts, sub); });
  }

  // sync hands an existing device over again, so the app hears of one the browser renewed.
  function sync(opts) {
    return state(opts).then(function (s) {
      if (s !== "on") return null;
      return existing(opts).then(function (sub) { return sub ? hand(opts, sub) : null; });
    });
  }

  function disable(opts) {
    return existing(opts).then(function (sub) {
      if (!sub) return null;
      var json = sub.toJSON();
      return sub.unsubscribe().then(function () {
        if (opts && typeof opts.unregister === "function") return opts.unregister(json);
      }).then(function () { return json; });
    });
  }

  window.Whisk = window.Whisk || {};
  window.Whisk.push = { state: state, enable: enable, sync: sync, disable: disable };
})();
