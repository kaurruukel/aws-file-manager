document.addEventListener("DOMContentLoaded", () => {
	fhl = document.querySelectorAll(".my-target");
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

	// opon the root folder
	document.getElementById("/").click()
})

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
