import{d as E,q as N,s as S,p as U,c as p,h as v,b as x,t as M,B as P,w as L,Y as O,k as R,f as D,l as u,v as q,m as w,D as T,R as V,o as d,i as $,_ as z}from"./index-RAH3Attl.js";const A={class:"plugin-ui-page"},J={class:"bar"},W={class:"page-title"},H={key:0,class:"state"},I={key:1,class:"state error"},Y=["title","srcdoc"],i="viewer-plugin-ui-v1",F=E({__name:"PluginUiPage",props:{name:{}},setup(B){const k=B,_=D(),m=u(null),r=u(null),f=u(""),g=u(!0),l=u("");function j(){return location.origin}function C(s,e){const t=JSON.stringify({name:e.name,title:e.title,baseUrl:j()}).replace(/</g,"\\u003c"),a=`<script>
    (() => {
      const channel = ${JSON.stringify(i)};
      const pending = new Map();
      let sequence = 0;
      addEventListener("message", (event) => {
        const value = event.data;
        if (event.source !== parent || !value || value.channel !== channel || value.type !== "response") return;
        const task = pending.get(value.id);
        if (!task) return;
        pending.delete(value.id);
        clearTimeout(task.timer);
        if (value.ok) task.resolve(value.data);
        else task.reject(new Error(value.error || "插件请求失败"));
      });
      window.viewerPlugin = Object.freeze({
        context: Object.freeze(${t}),
        send(type, payload) {
          return new Promise((resolve, reject) => {
            const id = String(Date.now()) + "-" + String(++sequence);
            const timer = setTimeout(() => {
              pending.delete(id);
              reject(new Error(type === "request" ? "插件请求超时" : "弹窗响应超时"));
            }, 65000);
            pending.set(id, { resolve, reject, timer });
            parent.postMessage({ channel, type, id, ...payload }, "*");
          });
        },
        request(method, path, body) {
          return this.send("request", { method, path, body });
        },
        alert(message, options) {
          return this.send("dialog", { kind: "alert", message, options });
        },
        confirm(message, options) {
          return this.send("dialog", { kind: "confirm", message, options });
        },
        close() { parent.postMessage({ channel, type: "close" }, "*"); },
      });
    })();
  <\/script>`,n=s.match(/<head(?:\s[^>]*)?>/i);if((n==null?void 0:n.index)!==void 0){const c=n.index+n[0].length;return s.slice(0,c)+a+s.slice(c)}return a+s}async function y(){g.value=!0,l.value="",r.value=null,f.value="";try{const s=await q.pluginUi(k.name);r.value=s,f.value=C(s.html,s)}catch(s){l.value=w(s)}finally{g.value=!1}}async function b(s){var a,n,c;if(s.source!==((a=m.value)==null?void 0:a.contentWindow))return;const e=s.data;if(!e||e.channel!==i)return;if(e.type==="close"){await _.push("/admin/plugins");return}if(typeof e.id!="string")return;const t=(n=m.value)==null?void 0:n.contentWindow;if(e.type==="dialog"){const o=e.options&&typeof e.options=="object"?e.options:{};try{const h=e.kind==="confirm"?await T(e.message,o):(await V(e.message,o),!0);t==null||t.postMessage({channel:i,type:"response",id:e.id,ok:!0,data:h},"*")}catch(h){t==null||t.postMessage({channel:i,type:"response",id:e.id,ok:!1,error:w(h)},"*")}return}if(e.type==="request")try{const o=await q.pluginUiRequest(((c=r.value)==null?void 0:c.apiBase)??null,String(e.method||""),String(e.path||""),e.body);t==null||t.postMessage({channel:i,type:"response",id:e.id,ok:!0,data:o},"*")}catch(o){t==null||t.postMessage({channel:i,type:"response",id:e.id,ok:!1,error:w(o)},"*")}}return N(()=>{window.addEventListener("message",b),y()}),S(()=>window.removeEventListener("message",b)),U(()=>k.name,y),(s,e)=>{var t,a;return d(),p("div",A,[v("div",J,[v("div",null,[v("button",{class:"back",type:"button",onClick:e[0]||(e[0]=n=>x(_).push("/admin/plugins"))},"← 插件管理"),v("h2",W,M(((t=r.value)==null?void 0:t.title)||"插件界面"),1)]),l.value?(d(),P(x(O),{key:0,onClick:y},{default:L(()=>[...e[1]||(e[1]=[$("重新加载",-1)])]),_:1})):R("",!0)]),g.value?(d(),p("p",H,"正在加载插件界面…")):l.value?(d(),p("p",I,M(l.value),1)):(d(),p("iframe",{key:2,ref_key:"frame",ref:m,class:"plugin-frame",title:((a=r.value)==null?void 0:a.title)||"插件界面",srcdoc:f.value,sandbox:"allow-scripts allow-forms"},null,8,Y))])}}}),K=z(F,[["__scopeId","data-v-ed097bb0"]]);export{K as default};
