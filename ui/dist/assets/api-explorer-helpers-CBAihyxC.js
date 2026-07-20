const p=["GET","POST","PATCH","DELETE"],l="ayb_api_explorer_history";const g={GET:"bg-green-100 text-green-700",POST:"bg-blue-100 text-blue-700",PATCH:"bg-yellow-100 text-yellow-700",DELETE:"bg-red-100 text-red-700"};function $(){try{const r=localStorage.getItem(l);return r?JSON.parse(r):[]}catch{return[]}}function E(r){localStorage.setItem(l,JSON.stringify(r.slice(0,20)))}function y(r){try{return JSON.stringify(JSON.parse(r),null,2)}catch{return r}}function H(r,a,t){let e=`curl -X ${r}`;return e+=` \\
  -H "Authorization: Bearer <TOKEN>"`,t&&(r==="POST"||r==="PATCH")&&(e+=` \\
  -H "Content-Type: application/json"`,e+=` \\
  -d '${t}'`),e+=` \\
  "${a}"`,e}function P(r,a,t){const e=a.match(/^\/api\/collections\/([^/?]+)(?:\/([^/?]+))?/);if(e){const s=e[1],n=e[2],u=a.includes("?")?a.split("?")[1]:"",f=new URLSearchParams(u);if(r==="GET"&&!n){const c=[];for(const[S,O]of f)c.push(`  ${S}: "${O}"`);const T=c.length>0?`, {
${c.join(`,
`)}
}`:"";return`const { items } = await ayb.records.list("${s}"${T});`}if(r==="GET"&&n)return`const record = await ayb.records.get("${s}", "${n}");`;if(r==="POST"){const c=t?JSON.parse(t):{};return`const record = await ayb.records.create("${s}", ${JSON.stringify(c,null,2)});`}if(r==="PATCH"&&n){const c=t?JSON.parse(t):{};return`const record = await ayb.records.update("${s}", "${n}", ${JSON.stringify(c,null,2)});`}if(r==="DELETE"&&n)return`await ayb.records.delete("${s}", "${n}");`}const o=a.match(/^\/api\/rpc\/([^/?]+)/);if(o&&r==="POST"){const s=o[1],n=t?JSON.parse(t):{};return`const result = await ayb.rpc("${s}", ${JSON.stringify(n,null,2)});`}let i=`const res = await fetch("${a}", {
  method: "${r}",
  headers: {
    "Authorization": "Bearer <TOKEN>"`;return t&&(r==="POST"||r==="PATCH")&&(i+=`,
    "Content-Type": "application/json"`),i+=`
  }`,t&&(r==="POST"||r==="PATCH")&&(i+=`,
  body: JSON.stringify(${t})`),i+=`
});
const data = await res.json();`,i}function w(r){return r>=200&&r<300?"text-green-600":r>=300&&r<400?"text-yellow-600":r>=400&&r<500?"text-orange-600":"text-red-600"}export{p as M,g as a,P as b,E as c,y as f,H as g,$ as l,w as s};
