const categoryOptions = document.getElementsByClassName("category-option");
const amountInput = document.getElementById("amount-input");
const sharedOption = document.querySelector(
  'input[name="sharing-choice"]:checked'
);

const serverURL = document.getElementById("server-url").innerText;

const successText = document.getElementById("success-text");

amountInput.addEventListener("input", (e) => {
  successText.innerText = "";
});

for (let option of categoryOptions) {
  console.log(option);

  option.addEventListener("click", (e) => {
    let category = option.getAttribute("data-category");
    fetch(
      `${serverURL}/v1/pay/${sharedOption.value}/${amountInput.value}/${category}`,
      {
        method: "POST",
      }
    )
      .then((res) => {
        if (!res.ok) {
          throw new Error("Network response was not ok: " + res.statusText);
        }

        return res.text();
      })
      .then((text) => {
        amountInput.value = "";
        successText.innerText = "Kjøpet ble lagt til!";
        console.log(text); // This will log the text content to the console
      });
  });
}
