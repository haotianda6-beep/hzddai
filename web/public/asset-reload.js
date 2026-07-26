;(function () {
  try {
    if (window.caches && caches.keys) {
      caches.keys().then(function (keys) {
        keys.forEach(function (key) {
          caches.delete(key)
        })
      })
    }
  } catch (_) {}
  try {
    var url = new URL(window.location.href)
    url.searchParams.set('_v', String(Date.now()))
    window.location.replace(url.toString())
  } catch (_) {
    window.location.reload()
  }
})()
