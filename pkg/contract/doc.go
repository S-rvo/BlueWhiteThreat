// Package contract est la source unique des noms partagés entre services :
// champs du document Mongo "article" et clés Redis échangées entre crawler et
// scraper. Un service ne doit pas réécrire ces chaînes à la main : il importe
// ces constantes, et ses tests de divergence échouent si son code s'en écarte.
//
// Le module n'a aucune dépendance pour rester importable par tous les services
// (même mécanisme require/replace que pkg/csamfilter).
package contract
