
window.onload = function() {
	var select = document.getElementById("select")
	if (select.selectedIndex != 0) {
		select.dispatchEvent(new Event("change"))
	}
}



document.addEventListener('htmx:afterOnLoad', function() {
	fhl = document.querySelectorAll(".my-target");
	if (fhl.length == 0) {
		return
	}
	fobj = {};
	for (let i = 0; i < fhl.length; i++) {
		fh = fhl[i];
		fh.addEventListener("click", (e) => {
			parent = e.target.parentNode
			t = parent.id
			if (fobj[t]) {
				parent.nextElementSibling.classList.remove('open')
			} else {
				parent.nextElementSibling.classList.add('open')
			}
			fobj[t] = !fobj[t]
		})
	}


});
function downloadFile(key) {
	fetch("/download-file?key=" + key)
		.then(async response => {
			var contentDisposition = response.headers.get('Content-Disposition');
			var filename = contentDisposition.split('filename=')[1].replace(/"/g, '');
			const blob = await response.blob();
			return ({ blob, filename });
		}
		)
		.then(({ blob, filename }) => {
			var url = window.URL.createObjectURL(blob);
			var a = document.createElement('a');
			a.href = url;
			a.download = filename;
			document.body.appendChild(a);
			a.click();
			a.remove();
		});
}

document.addEventListener("keydown", (e) => {
	if (e.key == "Escape") {
		document.querySelector("#overlay").style.display = "none";
	}
})

function hideOverlay() {
	document.querySelector("#overlay").style.display = "none";
}
